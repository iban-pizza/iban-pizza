package sources

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strings"
	"sync"

	"github.com/iban-pizza/iban-pizza/bankdata"
)

// Some registries list an institution's bank code and LEI but no BIC. The BIC
// is then joined from the BIC-to-LEI mapping that SWIFT and GLEIF publish
// together. That file is not CC0 like the rest of GLEIF's data: it has its own
// licence, which permits any use provided the SWIFT notice accompanies copies.

// BICMappingName is recorded on every BIC obtained through the join, so an
// answer shows that the BIC did not come from the national registry.
const BICMappingName = "GLEIF BIC-to-LEI mapping (SWIFT)"

// BICMappingTerms are the terms of the mapping file.
var BICMappingTerms = Terms{
	Licence: "BIC/LEI Mapping Table License Agreement",
	URL:     "https://www.gleif.org/en/lei-data/lei-mapping/download-bic-to-lei-relationship-files",
	Attribution: "SWIFT © and database rights. All rights reserved. This Mapping Table has been developed by SWIFT. " +
		"Any use of the Mapping Table, in whole or in part, is subject to the BIC/LEI Mapping Table License Agreement " +
		"as published with the Mapping Table available on GLEIF's website. The Mapping Table is updated monthly. " +
		"For the latest BIC information and updates, always refer to www.swift.com/bic.",
	Note: "Any use is permitted, commercial included, provided the notice accompanies every copy, in whole or in part.",
}

// bicMappingPage lists the monthly files, newest first. The download URL
// carries an identifier that changes with each release.
const bicMappingPage = "https://www.gleif.org/en/lei-data/lei-mapping/download-bic-to-lei-relationship-files"

var bicMappingLink = regexp.MustCompile(`(https://mapping\.gleif\.org/api/v2/bic-lei/[0-9a-f-]+/download)`)

// BICMapping resolves an LEI to the BICs registered for it.
type BICMapping struct {
	byLEI map[string][]string
}

// ParseBICMapping reads the mapping, either the zip archive GLEIF serves or
// the CSV inside it.
func ParseBICMapping(data []byte) (*BICMapping, error) {
	if bytes.HasPrefix(data, []byte("PK")) {
		zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
		if err != nil {
			return nil, fmt.Errorf("open mapping archive: %w", err)
		}
		data = nil
		for _, f := range zr.File {
			if !strings.HasSuffix(strings.ToLower(f.Name), ".csv") {
				continue
			}
			rc, err := f.Open()
			if err != nil {
				return nil, err
			}
			data, err = io.ReadAll(io.LimitReader(rc, MaxDownloadBytes))
			rc.Close()
			if err != nil {
				return nil, err
			}
			break
		}
		if data == nil {
			return nil, fmt.Errorf("mapping archive holds no csv file")
		}
	}

	table, err := parseCSV(DecodeAuto(data), ',', "lei", "bic")
	if err != nil {
		return nil, err
	}
	leiAt, err := table.mustIndex("LEI")
	if err != nil {
		return nil, err
	}
	bicAt, err := table.mustIndex("BIC")
	if err != nil {
		return nil, err
	}

	m := &BICMapping{byLEI: make(map[string][]string, len(table.rows))}
	for _, row := range table.rows {
		lei := strings.ToUpper(field(row, leiAt))
		bic := strings.ToUpper(field(row, bicAt))
		if len(lei) != 20 || (len(bic) != 8 && len(bic) != 11) {
			continue
		}
		m.byLEI[lei] = append(m.byLEI[lei], bic)
	}
	if len(m.byLEI) == 0 {
		return nil, fmt.Errorf("mapping holds no usable rows")
	}
	return m, nil
}

// Len reports how many LEIs the mapping covers.
func (m *BICMapping) Len() int { return len(m.byLEI) }

// Lookup returns the BIC of an institution in one country, or "" when the
// mapping does not identify exactly one.
//
// A large group holds many BICs under one LEI, in several countries and for
// several business lines. Only BICs carrying the wanted country code count,
// and the answer is given only when those agree on one institution code,
// because a guessed BIC routes a payment to the wrong place and a missing one
// does not. Among the matching BICs the head office form ending in XXX wins.
func (m *BICMapping) Lookup(lei, country string) string {
	country = strings.ToUpper(country)
	var local []string
	for _, bic := range m.byLEI[strings.ToUpper(strings.TrimSpace(lei))] {
		if bic[4:6] == country {
			local = append(local, bic)
		}
	}
	if len(local) == 0 {
		return ""
	}
	sort.Strings(local)
	for _, bic := range local[1:] {
		if bic[:8] != local[0][:8] {
			return ""
		}
	}
	for _, bic := range local {
		if len(bic) == 8 || strings.HasSuffix(bic, "XXX") {
			return bic
		}
	}
	return local[0]
}

// Fill sets the BIC on every bank that has an LEI but no BIC, and returns how
// many it resolved.
func (m *BICMapping) Fill(banks []bankdata.Bank) int {
	filled := 0
	for i := range banks {
		if banks[i].BIC != "" || banks[i].LEI == "" {
			continue
		}
		if bic := m.Lookup(banks[i].LEI, banks[i].Country); bic != "" {
			banks[i].BIC = bic
			banks[i].BICSource = BICMappingName
			filled++
		}
	}
	return filled
}

// needsBICs reports whether any record could gain a BIC from the mapping.
func needsBICs(banks []bankdata.Bank) bool {
	for _, b := range banks {
		if b.BIC == "" && b.LEI != "" {
			return true
		}
	}
	return false
}

// bicCache holds the mapping for one update run.
type bicCache struct {
	once    sync.Once
	mapping *BICMapping
	err     error
}

// BICMapping downloads the current mapping, once per client.
func (c *Client) BICMapping(ctx context.Context) (*BICMapping, error) {
	c.bics.once.Do(func() {
		url, err := c.ResolveLink(ctx, bicMappingPage, bicMappingLink)
		if err != nil {
			c.bics.err = err
			return
		}
		data, err := c.Get(ctx, url)
		if err != nil {
			c.bics.err = err
			return
		}
		c.bics.mapping, c.bics.err = ParseBICMapping(data)
	})
	return c.bics.mapping, c.bics.err
}
