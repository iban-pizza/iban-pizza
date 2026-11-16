package sources

import (
	"context"
	"regexp"
	"strings"
	"time"

	"github.com/iban-pizza/iban-pizza/bankdata"
)

func init() { Register(BancoDeEspana{}) }

// bancoDeEspanaURL is the register of institutions as a spreadsheet. The file
// name is stable and the file is rebuilt as the register changes.
const bancoDeEspanaURL = "https://www.bde.es/f/webbe/SGE/regis/ficheros/es/Registro_Entidades.xlsx"

// bancoDeEspanaSheet holds one row per institution; the other sheets list
// activities and cross border services.
const bancoDeEspanaSheet = "DATOS GENERALES"

// BancoDeEspana reads the Spanish register of institutions.
//
// The register keeps every institution it ever listed and covers every kind
// of supervised firm, so most rows are either closed or are no account
// providers at all. The Banco de España code is the bank code in the IBAN.
type BancoDeEspana struct{}

func (BancoDeEspana) Country() string { return "ES" }
func (BancoDeEspana) Name() string    { return "Banco de España" }

// Partial reports that an unknown bank code is not proof of a wrong one here.
func (BancoDeEspana) Partial() string {
	return "The register is filtered to the entity types that hold payment accounts, so a type the publisher adds or renames is missing until the filter is extended."
}

func (BancoDeEspana) Terms() Terms {
	return Terms{
		Licence:     "Banco de España aviso legal",
		URL:         "https://www.bde.es/wbe/es/pie/aviso-legal/",
		Attribution: "Fuente: Banco de España",
		Note:        "Reproduction is permitted with the source named, provided it is faithful and the content is not altered.",
	}
}

func (b BancoDeEspana) Fetch(ctx context.Context, c *Client) ([]byte, string, error) {
	data, err := c.Get(ctx, bancoDeEspanaURL)
	return data, bancoDeEspanaURL, err
}

// bancoDeEspanaTypes are the kinds of institution that hold payment accounts
// and so can appear in an IBAN: banks, savings banks, credit cooperatives,
// the state credit institute, branches of foreign credit institutions, and
// payment and electronic money institutions with their branches. Names are
// compared in lower case.
//
// The other kinds that carry a four digit code are left out on purpose:
// representative offices, which may not take business, appraisal companies,
// mutual guarantee and counter guarantee societies, currency exchange
// bureaux, holding companies, plain specialised lenders (establecimientos
// financieros de crédito, which lend but keep no accounts), branches of
// financial subsidiaries, and account information providers, which only read
// accounts held elsewhere. A kind that is not listed here is dropped, so a
// new kind of account provider has to be added by hand.
var bancoDeEspanaTypes = map[string]bool{
	"banco":                  true,
	"caja de ahorros":        true,
	"cooperativa de crédito": true,
	"crédito oficial":        true,
	"sucursal de entidad de crédito extranjera comunitaria":      true,
	"sucursal de entidad de crédito extranjera extracomunitaria": true,
	"entidad de pago": true,
	"entidad de pago exenta según art. 14 rdl 19/2018":                 true,
	"sucursal de entidad de pago extranjera comunitaria":               true,
	"entidad de dinero electrónico":                                    true,
	"sucursal de entidad de dinero electrónico extranjera comunitaria": true,
	"establecimiento financiero de crédito entidad de pago":            true,
}

// bancoDeEspanaCode is the shape of a code that can stand in an IBAN. Mortgage
// intermediaries and firms working across the border without a branch carry
// codes with letters and are no account providers.
var bancoDeEspanaCode = regexp.MustCompile(`^\d{4}$`)

func (b BancoDeEspana) Parse(data []byte) ([]bankdata.Bank, error) {
	rows, err := readXLSX(data, bancoDeEspanaSheet)
	if err != nil {
		return nil, err
	}
	// The header is found by content because a date banner sits above it.
	table, err := tableFromRows(rows, "CÓDIGO BE", "NOMBRE", "FECHA BAJA")
	if err != nil {
		return nil, err
	}
	codeAt, err := table.mustIndex("CÓDIGO BE")
	if err != nil {
		return nil, err
	}
	nameAt, err := table.mustIndex("NOMBRE")
	if err != nil {
		return nil, err
	}
	typeAt, err := table.mustIndex("TIPO ENTIDAD")
	if err != nil {
		return nil, err
	}
	// Without this column closed institutions could not be told apart, so its
	// absence is an error rather than something to work around.
	closedAt, err := table.mustIndex("FECHA BAJA")
	if err != nil {
		return nil, err
	}
	leiAt, hasLEI := table.index("CÓDIGO LEI")
	zipAt, hasZip := table.index("CÓDIGO POSTAL")
	cityAt, hasCity := table.index("POBLACIÓN")

	var out []bankdata.Bank
	seen := map[string]bool{}
	for _, row := range table.rows {
		code := field(row, codeAt)
		// A code stored as a number loses its leading zeros.
		if code != "" && len(code) < 4 && strings.Trim(code, "0123456789") == "" {
			code = strings.Repeat("0", 4-len(code)) + code
		}
		name := field(row, nameAt)
		if !bancoDeEspanaCode.MatchString(code) || name == "" || seen[code] {
			continue
		}
		// A row with a closing date is an institution that no longer exists;
		// its code may since have been absorbed by another.
		if field(row, closedAt) != "" {
			continue
		}
		if !bancoDeEspanaTypes[strings.ToLower(field(row, typeAt))] {
			continue
		}
		seen[code] = true

		bank := bankdata.Bank{
			Country:  "ES",
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
		out = append(out, bank)
	}
	return out, nil
}

var bancoDeEspanaDate = regexp.MustCompile(`FECHA:\s*(\d{2}/\d{2}/\d{4})`)

// PublishedAt reads the "FECHA: dd/mm/yyyy" banner above the header.
func (BancoDeEspana) PublishedAt(data []byte) time.Time {
	rows, err := readXLSX(data, bancoDeEspanaSheet)
	if err != nil {
		return time.Time{}
	}
	for _, row := range rows[:min(len(rows), 5)] {
		if m := bancoDeEspanaDate.FindStringSubmatch(strings.Join(row, " ")); m != nil {
			if t, err := time.Parse("02/01/2006", m[1]); err == nil {
				return t
			}
		}
	}
	return time.Time{}
}
