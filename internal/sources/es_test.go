package sources

import "testing"

func TestBancoDeEspanaParse(t *testing.T) {
	// Invented rows in the layout of the published sheet.
	data := buildXLSX(t, "DATOS GENERALES", [][]string{
		{"FECHA: 01/09/2026"},
		{"CÓDIGO BE", "NOMBRE", "TIPO ENTIDAD", "CÓDIGO LEI", "FECHA ALTA", "POBLACIÓN", "CÓDIGO POSTAL", "FECHA BAJA", "MOTIVO BAJA"},
		{"9901", "BANCO EJEMPLO, S.A.", "Banco", "AAAAAAAAAAAAAAAAAA01", "01/01/1990", "MADRID", "28001", "", ""},
		// A numeric cell has lost its leading zeros.
		{"#77", "CAJA RURAL DE EJEMPLO", "Cooperativa de crédito", "", "01/01/1990", "SEVILLA", "41001", "", ""},
		// Closed: the code must not resolve to a bank that no longer exists.
		{"9902", "BANCO CERRADO, S.A.", "Banco", "", "01/01/1950", "BILBAO", "48001", "19/08/1988", "Por fusión por absorción por"},
		{"9903", "BANCO DE PRUEBA, SUCURSAL EN ESPAÑA", "Sucursal de entidad de crédito extranjera comunitaria", "AAAAAAAAAAAAAAAAAA03", "01/01/2010", "MADRID", "28002", "", ""},
		{"9904", "PAGOS EJEMPLO, E.P., S.A.", "Entidad de pago", "", "01/01/2015", "VALENCIA", "46001", "", ""},
		{"9905", "DINERO EJEMPLO, E.D.E., S.L.", "Entidad de dinero electrónico", "", "01/01/2016", "MADRID", "28003", "", ""},
		// Active four digit rows that are no account providers.
		{"9906", "OFICINA DE REPRESENTACION DE BANCO EJEMPLO", "Oficina de representación en España de entidad de crédito extranjera", "", "01/01/2000", "MADRID", "28004", "", ""},
		{"9907", "TASACIONES EJEMPLO, S.A.", "Sociedad de tasación", "", "01/01/2000", "MADRID", "28005", "", ""},
		{"9908", "GARANTIAS EJEMPLO, S.G.R.", "Sociedad de garantía reciproca", "", "01/01/2000", "MADRID", "28006", "", ""},
		// Not a four digit code.
		{"D123", "INTERMEDIARIO EJEMPLO, S.L.", "Intermediario de crédito inmobiliario", "", "01/01/2020", "MADRID", "28007", "", ""},
		// A second row for a code already taken.
		{"9901", "BANCO EJEMPLO DUPLICADO, S.A.", "Banco", "", "01/01/1990", "MADRID", "28001", "", ""},
	})

	banks, err := BancoDeEspana{}.Parse(data)
	if err != nil {
		t.Fatalf("Parse returned %v", err)
	}
	got := codeIndex(banks)
	if len(banks) != 5 || len(got) != 5 {
		t.Fatalf("got %d records %v, want 5", len(banks), got)
	}
	if b := got["9901"]; b.Name != "BANCO EJEMPLO, S.A." || b.LEI != "AAAAAAAAAAAAAAAAAA01" || b.City != "MADRID" || b.Zip != "28001" || b.Country != "ES" || b.BIC != "" {
		t.Errorf("9901 = %+v", b)
	}
	if b := got["0077"]; b.Name != "CAJA RURAL DE EJEMPLO" || b.LEI != "" {
		t.Errorf("0077 = %+v", b)
	}
	if b := got["9903"]; b.LEI != "AAAAAAAAAAAAAAAAAA03" {
		t.Errorf("9903 = %+v", b)
	}
	for _, code := range []string{"9904", "9905"} {
		if _, ok := got[code]; !ok {
			t.Errorf("%s missing, payment and electronic money institutions must be kept", code)
		}
	}
	for _, code := range []string{"9902", "9906", "9907", "9908", "D123"} {
		if b, ok := got[code]; ok {
			t.Errorf("%s was emitted: %+v", code, b)
		}
	}

	if d := (BancoDeEspana{}).PublishedAt(data); d.Format("2006-01-02") != "2026-09-01" {
		t.Errorf("PublishedAt = %v, want 2026-09-01", d)
	}
}
