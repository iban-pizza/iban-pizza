package sources

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"

	"github.com/iban-pizza/iban-pizza/bankdata"
)

func init() { Register(BancaDItalia{}) }

// bancaDItaliaURL is the export of the supervisory registers. The anonymous
// user name is the one the public download page itself sends.
const bancaDItaliaURL = "https://infostat.bancaditalia.it/GIAVAInquiry-public/getAreaDownloadExport.do" +
	"?referenceDate=&product=VFLUSSO_INTERMEDIARIO&language=IT&exportType=CSV&username=OBLIXANONYMOUS&isCompressed=S"

// BancaDItalia reads the Italian list of supervised intermediaries.
//
// The file is the full history of every register the Banca d'Italia keeps, so
// one institution appears once for each change of name or seat and once more
// for each register it belongs to. The ABI code is the bank code in the IBAN.
type BancaDItalia struct{}

func (BancaDItalia) Country() string { return "IT" }
func (BancaDItalia) Name() string    { return "Banca d'Italia" }

// Partial reports that an unknown bank code is not proof of a wrong one here.
func (BancaDItalia) Partial() string {
	return "The registers list supervised institutions; BancoPosta, whose code 07601 is in wide use, is not among them."
}

func (BancaDItalia) Terms() Terms {
	return Terms{
		Licence:     "CC BY 4.0",
		URL:         "https://www.bancaditalia.it/footer/copyright/index.html",
		Attribution: "Source: Banca d'Italia, Albi ed elenchi di vigilanza, CC BY 4.0",
	}
}

// Fetch relies on the client keeping cookies: the host answers the first
// request with a session cookie and a redirect, and serves the file only when
// the cookie comes back.
func (b BancaDItalia) Fetch(ctx context.Context, c *Client) ([]byte, string, error) {
	data, err := c.Get(ctx, bancaDItaliaURL)
	return data, bancaDItaliaURL, err
}

// bancaDItaliaOpen is the end date of a row that is still in force.
const bancaDItaliaOpen = "9999-12-31"

// bancaDItaliaRegisters lists the registers whose members hold payment
// accounts and so can appear in an IBAN, in order of preference: banks, then
// payment institutions, then electronic money institutions. A register is
// matched by the start of its name.
//
// Everything else in the file is left out on purpose: asset managers and
// investment companies (the registers under art. 35 TUF, SICAF, EuVECA and
// ELTIF managers), financial intermediaries under art. 106 TUB that lend but
// keep no accounts, trust companies, microcredit operators, credit servicers
// and account information providers, which only read accounts held elsewhere.
// The account information section is named "ALBO DEGLI IP - SEZIONE
// SPECIALE", which the "ALBO IP" prefix deliberately does not match.
var bancaDItaliaRegisters = []string{
	"ALBO DELLE BANCHE",
	"ALBO IP",
	"ALBO IMEL",
}

func (b BancaDItalia) Parse(data []byte) ([]bankdata.Bank, error) {
	csvData, _, err := bancaDItaliaCSV(data)
	if err != nil {
		return nil, err
	}
	// The first header starts with a space and an empty cell is a single
	// space; header matching and field both trim, so neither needs a fix here.
	table, err := parseCSV(DecodeAuto(csvData), ';', "COD_MECC", "DEN_INT")
	if err != nil {
		return nil, err
	}
	codeAt, err := table.mustIndex("COD_MECC")
	if err != nil {
		return nil, err
	}
	nameAt, err := table.mustIndex("DEN_INT")
	if err != nil {
		return nil, err
	}
	registerAt, err := table.mustIndex("DES_TIPO_ALBO")
	if err != nil {
		return nil, err
	}
	endAt, err := table.mustIndex("DATA_F_VAL")
	if err != nil {
		return nil, err
	}
	leiAt, hasLEI := table.index("COD_LEI")
	zipAt, hasZip := table.index("CAP_SL")
	cityAt, hasCity := table.index("DES_COM_ITA_SL")

	// An institution listed in two registers, such as a lender that is also a
	// payment institution, must come out once, under the register that best
	// describes its accounts.
	best := map[string]int{}
	byCode := map[string]bankdata.Bank{}
	var order []string
	for _, row := range table.rows {
		// Superseded rows describe a former name, seat or registration.
		if field(row, endAt) != bancaDItaliaOpen {
			continue
		}
		rank := -1
		register := strings.ToUpper(field(row, registerAt))
		for i, prefix := range bancaDItaliaRegisters {
			if strings.HasPrefix(register, prefix) {
				rank = i
				break
			}
		}
		code, name := field(row, codeAt), field(row, nameAt)
		if rank < 0 || name == "" || code == "" || len(code) > 5 || strings.Trim(code, "0123456789") != "" {
			continue
		}
		// The file stores the ABI code as a number, without leading zeros.
		code = strings.Repeat("0", 5-len(code)) + code

		previous, known := best[code]
		if known && previous <= rank {
			continue
		}
		if !known {
			order = append(order, code)
		}
		best[code] = rank

		bank := bankdata.Bank{
			Country:  "IT",
			BankCode: code,
			Name:     name,
			Source:   b.Name(),
		}
		if hasLEI {
			if lei := strings.ToUpper(field(row, leiAt)); len(lei) == 20 {
				bank.LEI = lei
			}
		}
		if hasZip {
			bank.Zip = field(row, zipAt)
		}
		if hasCity {
			bank.City = field(row, cityAt)
		}
		byCode[code] = bank
	}

	out := make([]bankdata.Bank, 0, len(order))
	for _, code := range order {
		out = append(out, byCode[code])
	}
	return out, nil
}

var bancaDItaliaDate = regexp.MustCompile(`^(\d{4}-\d{2}-\d{2})_`)

// PublishedAt reads the reference date from the name of the file inside the
// archive, which starts with it.
func (BancaDItalia) PublishedAt(data []byte) time.Time {
	_, name, err := bancaDItaliaCSV(data)
	if err != nil {
		return time.Time{}
	}
	if m := bancaDItaliaDate.FindStringSubmatch(name); m != nil {
		if t, err := time.Parse("2006-01-02", m[1]); err == nil {
			return t
		}
	}
	return time.Time{}
}

// bancaDItaliaCSV returns the CSV inside the downloaded archive together with
// its file name. Data that is not an archive is taken to be the CSV itself.
func bancaDItaliaCSV(data []byte) ([]byte, string, error) {
	if !bytes.HasPrefix(data, []byte("PK")) {
		return data, "", nil
	}
	data = bancaDItaliaRepair(data)
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, "", fmt.Errorf("open archive: %w", err)
	}
	for _, f := range zr.File {
		if !strings.HasSuffix(strings.ToLower(f.Name), ".csv") {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return nil, "", fmt.Errorf("open %s: %w", f.Name, err)
		}
		// Bounded so that a crafted archive cannot expand without limit.
		raw, err := io.ReadAll(io.LimitReader(rc, MaxDownloadBytes+1))
		rc.Close()
		if err != nil {
			return nil, "", fmt.Errorf("read %s: %w", f.Name, err)
		}
		if len(raw) > MaxDownloadBytes {
			return nil, "", fmt.Errorf("%s exceeds the %d byte limit", f.Name, MaxDownloadBytes)
		}
		return raw, f.Name, nil
	}
	return nil, "", fmt.Errorf("archive holds no csv file")
}

// bancaDItaliaRepair completes an archive whose trailing comment is shorter
// than its own length field says.
//
// The export ends one byte early: the end record announces a 13 byte comment
// and 12 follow. Lenient tools shrug that off, but archive/zip takes the
// record for a false match and rejects the whole file. Padding the comment to
// its announced length makes the archive well formed without touching any of
// the data in it. A file that is already complete is returned unchanged.
func bancaDItaliaRepair(data []byte) []byte {
	const recordLen = 22 // fixed part of the end of central directory record
	at := bytes.LastIndex(data, []byte("PK\x05\x06"))
	if at < 0 || at+recordLen > len(data) {
		return data
	}
	comment := int(data[at+recordLen-2]) | int(data[at+recordLen-1])<<8
	missing := at + recordLen + comment - len(data)
	if missing <= 0 {
		return data
	}
	// Copy rather than append in place, so the caller's bytes stay as fetched.
	fixed := make([]byte, len(data)+missing)
	copy(fixed, data)
	return fixed
}
