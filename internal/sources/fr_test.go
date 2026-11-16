package sources

import "testing"

func TestACPRParse(t *testing.T) {
	// Invented rows in the layout of the published export, BOM included.
	data := []byte("\xEF\xBB\xBF" +
		"id_referentiel;cib;lei;type_entite;denomination;code_postal;ville;pays;categorie;evenements\r\n" +
		// A natural person acting as an agent: no code, never emitted.
		"1;[];;Personne physique;DUPONT Camille;75001;PARIS;FRANCE;Agent PSP;\r\n" +
		"2;\"[{\"\"code\"\": \"\"99001\"\", \"\"date\"\": \"\"1990-01-01\"\"}]\";AAAAAAAAAAAAAAAAAA01;Personne morale;Banque Exemple;75002;PARIS;FRANCE;Établissement de Crédit;\r\n" +
		// Several codes in one cell, and an event text that spans two lines.
		"3;\"[{\"\"code\"\": \"\"99002\"\", \"\"date\"\": \"\"1970-01-01\"\"}, {\"\"code\"\": \"\"00703\"\", \"\"date\"\": \"\"2020-01-01\"\"}]\";;Personne morale;Banque Test;69001;LYON;FRANCE;Établissement de Crédit;\"Fusion le 1er janvier.\nSuite; sans objet.\"\r\n" +
		// A code stored as a number has lost its leading zeros.
		"4;\"[{\"\"code\"\": 704, \"\"date\"\": \"\"2021-01-01\"\"}]\";;Personne morale;Paiement Exemple;13001;MARSEILLE;FRANCE;Etablissement de paiement;\r\n" +
		"5;;;Personne morale;Courtier Exemple;31000;TOULOUSE;FRANCE;Agent PSP;\r\n")

	banks, err := ACPR{}.Parse(data)
	if err != nil {
		t.Fatalf("Parse returned %v", err)
	}
	got := codeIndex(banks)
	if len(banks) != 4 || len(got) != 4 {
		t.Fatalf("got %d records %v, want 4 (rows without a code must be dropped)", len(banks), got)
	}
	if b := got["99001"]; b.Name != "Banque Exemple" || b.LEI != "AAAAAAAAAAAAAAAAAA01" || b.Zip != "75002" || b.City != "PARIS" || b.Country != "FR" || b.BIC != "" {
		t.Errorf("99001 = %+v", b)
	}
	// Both codes of one institution resolve to it, and the line break inside
	// the event text did not split the row.
	for _, code := range []string{"99002", "00703"} {
		if b := got[code]; b.Name != "Banque Test" || b.City != "LYON" || b.LEI != "" {
			t.Errorf("%s = %+v", code, b)
		}
	}
	if b := got["00704"]; b.Name != "Paiement Exemple" {
		t.Errorf("00704 = %+v", b)
	}
	for _, b := range banks {
		if b.Name == "DUPONT Camille" || b.Name == "Courtier Exemple" {
			t.Errorf("row without a code was emitted: %+v", b)
		}
	}
}
