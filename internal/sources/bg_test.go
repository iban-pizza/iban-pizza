package sources

import "testing"

// bnbPage is an invented page in the layout of the published one: the header
// in a table of its own and repeated, one table per bank, the branch lists
// nested in a row of an outer table, and a retired register with an older
// date left inside an HTML comment.
const bnbPage = `<html><head><title>(untitled)</title></head><body>
<table><tr><td>Тази страница използва бисквитки.</td></tr></table>
<h2>Регистър на БАЕ кодовете</h2>
<h2>Регистър на БАЕ кодовете, предоставени от БНБ на доставчици на платежни услуги (ДПУ) от  07.02.2026</h2>
<div id="bae">
  <table><tr><th>Наименование на ДПУ </th><th>БАЕ код</th><th>BIC код (ЦУ)</th></tr></table>
  <table><tr><td class="first">Примерна банка АД</td><td align="right">EXAM9660</td><td align="right">exam bg sf</td></tr></table>
  <table class="clicktable"><tr><td colspan="3">
    <table>
      <tr><td class="first">Примерна банка АД, кл. Варна</td><td align="right">EXAM7001</td><td align="right"></td></tr>
      <tr><td class="first">Примерна банка АД, кл. Пловдив</td><td align="right">EXAM7002</td><td align="right">&nbsp;</td></tr>
    </table>
  </td></tr></table>
  <table>
    <tr><td class="first">Тестова банка АД, кл. Русе</td><td align="right">TEST8001</td><td align="right"></td></tr>
    <tr><td class="first">Тестова банка АД</td><td align="right">TEST9000</td><td align="right">TESTBG22</td></tr>
    <tr><td class="first">Тестова банка АД, втори код</td><td align="right">TEST5000</td><td align="right">TESTBG22XXX</td></tr>
  </table>
  <table><tr><th>ДПУ, който не е банка</th><th>БАЕ код</th><th>BIC код (ЦУ)</th></tr></table>
  <table>
    <tr><td class="first">Примерни плащания АД, клон</td><td align="right">PAYX4001</td><td align="right"></td></tr>
    <tr><td class="first">Примерни плащания АД, втори клон</td><td align="right">PAYX4002</td><td align="right"></td></tr>
  </table>
</div>
<h2>Регистър на БАЕ кодовете, предоставени от БНБ на други ДПУ, от 15.03.2018</h2>
<!--<div id="bae">
  <table><tr><th>ДПУ, който не е банка</th><th>БАЕ код</th><th>BIC код</th></tr></table>
  <table><tr><td class="first">Стар доставчик АД</td><td align="right">OLDX4001</td><td align="right">OLDXBGSF</td></tr></table>
</div>-->
</body></html>`

func TestBNBParse(t *testing.T) {
	banks, err := BNB{}.Parse([]byte(bnbPage))
	if err != nil {
		t.Fatalf("Parse returned %v", err)
	}
	got := codeIndex(banks)
	if len(banks) != 3 {
		t.Fatalf("got %d records %v, want 3 (one per four letter code, none from the comment)", len(banks), got)
	}
	// Head office first, branches in a nested table after it.
	if b := got["EXAM"]; b.Name != "Примерна банка АД" || b.BIC != "EXAMBGSF" || b.Country != "BG" {
		t.Errorf("EXAM = %+v", b)
	}
	// A branch listed before the head office must not lend the bank its
	// name, and the first row with a BIC wins over a later one.
	if b := got["TEST"]; b.Name != "Тестова банка АД" || b.BIC != "TESTBG22" {
		t.Errorf("TEST = %+v", b)
	}
	// No row of this code has a BIC: the first name stands.
	if b := got["PAYX"]; b.Name != "Примерни плащания АД, клон" || b.BIC != "" {
		t.Errorf("PAYX = %+v", b)
	}

	// The current register's date, not the one of the retired register.
	if d := (BNB{}).PublishedAt([]byte(bnbPage)); d.Format("2006-01-02") != "2026-02-07" {
		t.Errorf("PublishedAt = %v, want 2026-02-07", d)
	}
	if d := (BNB{}).PublishedAt([]byte("<h2>Регистър на БАЕ кодовете</h2>")); !d.IsZero() {
		t.Errorf("PublishedAt without a date = %v, want zero", d)
	}
}
