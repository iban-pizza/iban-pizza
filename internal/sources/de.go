package sources

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/iban-pizza/iban-pizza/bankdata"
)

func init() { Register(Bundesbank{}) }

// bundesbankPage is the landing page that carries the current download links.
const bundesbankPage = "https://www.bundesbank.de/de/aufgaben/unbarer-zahlungsverkehr/" +
	"serviceangebot/bankleitzahlen/download-bankleitzahlen-602592"

// bundesbankLink matches the fixed width text export. The path contains a
// content hash that changes with every quarterly release, so the link is
// resolved from the page instead of being hard coded.
var bundesbankLink = regexp.MustCompile(`href="([^"]*blz-aktuell-txt-data\.txt)"`)

// Bundesbank reads the German bank code directory.
//
// The file is fixed width, ISO-8859-1 encoded and uses CRLF. Column positions
// are counted in characters, not bytes, which is why the rows are decoded
// before they are sliced.
type Bundesbank struct{}

func (Bundesbank) Country() string { return "DE" }
func (Bundesbank) Name() string    { return "Deutsche Bundesbank" }

func (b Bundesbank) Fetch(ctx context.Context, c *Client) ([]byte, string, error) {
	url, err := c.ResolveLink(ctx, bundesbankPage, bundesbankLink)
	if err != nil {
		return nil, "", err
	}
	data, err := c.Get(ctx, url)
	return data, url, err
}

// Field offsets in the Bundesbank record, in characters.
const (
	deBankCode  = 0
	deFeature   = 8  // "1" marks an institution with its own bank code
	deName      = 9  // 58 characters
	deZip       = 67 // 5
	deCity      = 72 // 35
	deShortName = 107
	dePAN       = 134
	deBIC       = 139 // 11
	deCheckAlgo = 150 // 2
	deRecordLen = 152 // everything up to here must be present
)

func (b Bundesbank) Parse(data []byte) ([]bankdata.Bank, error) {
	text := DecodeAuto(data)
	lines := SplitLines(text)
	if len(lines) == 0 {
		return nil, fmt.Errorf("file is empty")
	}

	out := make([]bankdata.Bank, 0, len(lines))
	for n, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		r := []rune(line)
		if len(r) < deRecordLen {
			return nil, fmt.Errorf("line %d is %d characters, want at least %d",
				n+1, len(r), deRecordLen)
		}

		// Branches repeat their head office bank code without carrying payment
		// data of their own. Keeping them would overwrite the head office
		// record, since both share the same key.
		if runeSlice(r, deFeature, deFeature+1) != "1" {
			continue
		}

		code := runeSlice(r, deBankCode, deFeature)
		if code == "" {
			continue
		}

		out = append(out, bankdata.Bank{
			Country:   "DE",
			BankCode:  code,
			Name:      runeSlice(r, deName, deZip),
			ShortName: runeSlice(r, deShortName, dePAN),
			Zip:       runeSlice(r, deZip, deCity),
			City:      runeSlice(r, deCity, deShortName),
			BIC:       strings.ReplaceAll(runeSlice(r, deBIC, deCheckAlgo), " ", ""),
			CheckAlgo: runeSlice(r, deCheckAlgo, deRecordLen),
			Source:    b.Name(),
		})
	}
	return out, nil
}
