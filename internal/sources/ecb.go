package sources

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"regexp"
	"strings"
	"unicode/utf16"
)

// The European Central Bank publishes one list of every monetary financial
// institution in the Union. It identifies an institution by its RIAD code,
// which each national central bank forms in its own way. Where that code
// embeds the national bank code, the list can stand in for a national
// registry that is not published in a readable form. The reader below is
// shared; what a RIAD code means is left to the country that uses it.

// ecbName is the source name recorded for records taken from the list.
const ecbName = "European Central Bank (MFI list)"

// ecbTerms are the terms the ECB attaches to its publications.
var ecbTerms = Terms{
	Licence:     "ECB copyright and reuse policy",
	URL:         "https://www.ecb.europa.eu/services/using-our-site/disclaimer/html/index.en.html",
	Attribution: "Source: European Central Bank, list of monetary financial institutions",
	Note:        "Free use is permitted provided the source is named; modifications must be stated.",
}

// ecbFeed announces each release of the ECB's market information service,
// newest first. The list is published under a path that changes with every
// release, so it has to be found through the feed.
const ecbFeed = "https://mid.ecb.europa.eu/rss/mid.xml"

var (
	// ecbFeedItem captures the release document of the first, and so newest,
	// feed item for the list.
	ecbFeedItem = regexp.MustCompile(`<title>\s*List of monetary financial institutions[^<]*</title>\s*<link>\s*([^<\s]+)\s*</link>`)
	// ecbReleaseFile captures the CSV attachment of a release, which also
	// offers the list in other formats.
	ecbReleaseFile = regexp.MustCompile(`<escb:linkFile>\s*([^<\s]+/mfi_csv_\d+\.csv\.gz)\s*</escb:linkFile>`)
)

// ecbFetch downloads the current list and returns it with the URL it used.
func ecbFetch(ctx context.Context, c *Client) ([]byte, string, error) {
	release, err := c.ResolveLink(ctx, ecbFeed, ecbFeedItem)
	if err != nil {
		return nil, ecbFeed, err
	}
	url, err := c.ResolveLink(ctx, release, ecbReleaseFile)
	if err != nil {
		return nil, release, err
	}
	data, err := c.Get(ctx, url)
	return data, url, err
}

// ecbInstitution is one row of the list.
type ecbInstitution struct {
	RIAD string // identifier, the country code followed by a national part
	LEI  string
	Name string
	Zip  string
	City string
}

// ecbInstitutions returns the rows of the list registered in one country.
func ecbInstitutions(data []byte, country string) ([]ecbInstitution, error) {
	text, err := ecbText(data)
	if err != nil {
		return nil, err
	}
	table, err := parseCSV(text, '\t', "RIAD_CODE", "COUNTRY_OF_REGISTRATION")
	if err != nil {
		return nil, err
	}
	riadAt, err := table.mustIndex("RIAD_CODE")
	if err != nil {
		return nil, err
	}
	countryAt, err := table.mustIndex("COUNTRY_OF_REGISTRATION")
	if err != nil {
		return nil, err
	}
	nameAt, err := table.mustIndex("NAME")
	if err != nil {
		return nil, err
	}
	leiAt, hasLEI := table.index("LEI")
	zipAt, hasZip := table.index("POSTAL")
	cityAt, hasCity := table.index("CITY")

	var out []ecbInstitution
	for _, row := range table.rows {
		if !strings.EqualFold(field(row, countryAt), country) {
			continue
		}
		in := ecbInstitution{
			RIAD: strings.ToUpper(field(row, riadAt)),
			Name: field(row, nameAt),
		}
		if in.RIAD == "" || in.Name == "" {
			continue
		}
		if hasLEI {
			if lei := strings.ToUpper(field(row, leiAt)); len(lei) == 20 {
				in.LEI = lei
			}
		}
		if hasZip {
			in.Zip = field(row, zipAt)
		}
		if hasCity {
			in.City = field(row, cityAt)
		}
		out = append(out, in)
	}
	return out, nil
}

// ecbText unpacks and decodes the list. It is published gzipped and in
// UTF-16 with a byte order mark; reading those bytes as UTF-8 would yield
// nothing usable, not even the header. Data that carries no mark is taken to
// be UTF-8 or Latin-1, in case the encoding changes.
func ecbText(data []byte) (string, error) {
	if bytes.HasPrefix(data, []byte{0x1F, 0x8B}) {
		zr, err := gzip.NewReader(bytes.NewReader(data))
		if err != nil {
			return "", fmt.Errorf("open gzip: %w", err)
		}
		// Bounded so that a crafted file cannot expand without limit.
		raw, err := io.ReadAll(io.LimitReader(zr, MaxDownloadBytes+1))
		if err != nil {
			return "", fmt.Errorf("read gzip: %w", err)
		}
		if len(raw) > MaxDownloadBytes {
			return "", fmt.Errorf("list exceeds the %d byte limit when unpacked", MaxDownloadBytes)
		}
		data = raw
	}

	var order binary.ByteOrder
	switch {
	case bytes.HasPrefix(data, []byte{0xFF, 0xFE}):
		order = binary.LittleEndian
	case bytes.HasPrefix(data, []byte{0xFE, 0xFF}):
		order = binary.BigEndian
	default:
		return DecodeAuto(data), nil
	}
	data = data[2:]
	units := make([]uint16, len(data)/2)
	for i := range units {
		units[i] = order.Uint16(data[2*i:])
	}
	return string(utf16.Decode(units)), nil
}
