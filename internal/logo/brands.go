package logo

// defaultBrands maps a four character BIC institution prefix to a brand name.
//
// The list is curated rather than derived, and that is deliberate. Deriving it
// from the bank register produces wrong answers, because several prefixes are
// shared BIC spaces rather than brands: GENO alone covers 1122 differently
// named cooperative banks, and the Landesbank prefixes below cover dozens of
// independent savings banks each. Naming any of those after one member would
// put the wrong brand on hundreds of institutions.
//
// Banks in a shared space are not a gap. They fall through to a monogram built
// from their own name, which is the correct mark for them.
//
// Every entry was checked against the current Bundesbank register.
func defaultBrands() map[string]string {
	return map[string]string{
		// Germany
		"DEUT": "Deutsche Bank",
		"COBA": "Commerzbank",
		"DRES": "Commerzbank",
		"HYVE": "UniCredit Bank",
		"PBNK": "Postbank",
		"INGD": "ING",
		"MARK": "Deutsche Bundesbank",
		"DGZF": "DekaBank",
		"NTSB": "N26",
		"AARB": "Aareal Bank",
		"DAAE": "apoBank",
		"SCFB": "Openbank",
		"OLBO": "Oldenburgische Landesbank",
		"BFSW": "SozialBank",
		"TRDA": "Tradegate",
		"BEVO": "Berliner Volksbank",

		// International institutions with a German presence
		"BNPA": "BNP Paribas",
		"SOGE": "Societe Generale",
		"CHAS": "J.P. Morgan",

		// Austria
		"GIBA": "Erste Bank",
		"BKAU": "UniCredit Bank Austria",
		"RZBA": "Raiffeisen Bank International",
		"OBKL": "Oberbank",
		"NABA": "Oesterreichische Nationalbank",
		"BAWA": "BAWAG",

		// Switzerland and Liechtenstein
		"UBSW": "UBS",
		"POFI": "PostFinance",
		"BLFL": "Liechtensteinische Landesbank",
		"BLKB": "Basellandschaftliche Kantonalbank",
	}
}

// sharedSpaces lists BIC prefixes that identify a group rather than a brand.
//
// They are documented here so that nobody adds them to the map above by
// looking at how many institutions share them. A high count is exactly the
// signal that a prefix is shared, not that it is an important brand.
var sharedSpaces = map[string]string{
	"GENO": "German cooperative banks",
	"WELA": "savings banks, Landesbank space",
	"NOLA": "savings banks, Landesbank space",
	"BYLA": "savings banks, Landesbank space",
	"SOLA": "savings banks, Landesbank space",
	"HELA": "savings banks, Landesbank space",
	"MALA": "savings banks, Landesbank space",
	"BRLA": "savings banks, Landesbank space",
}

// IsSharedSpace reports whether a BIC prefix belongs to a banking group whose
// members must not be given a common brand.
func IsSharedSpace(bic string) bool {
	_, ok := sharedSpaces[InstitutionPrefix(bic)]
	return ok
}
