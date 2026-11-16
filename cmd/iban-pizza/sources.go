package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/iban-pizza/iban-pizza/internal/sources"
)

// runSources prints every registry "iban-pizza update" downloads from, with
// the terms its publisher attaches.
//
// No bank data ships with the program, so the operator who runs the update is
// the one who obtains the data and is bound by those terms. This is where they
// can be read before anything is downloaded.
func runSources(args []string) error {
	fs := flag.NewFlagSet("sources", flag.ExitOnError)
	if err := fs.Parse(args); err != nil {
		return err
	}

	out := os.Stdout
	fmt.Fprintln(out, `"iban-pizza update" downloads from these publishers. Their terms apply to the`)
	fmt.Fprintln(out, "data you download; nothing below is redistributed by this program.")

	print := func(heading string, t sources.Terms) {
		fmt.Fprintf(out, "\n%s\n", heading)
		fmt.Fprintf(out, "  terms:        %s\n", orDash(t.Licence))
		fmt.Fprintf(out, "  published at: %s\n", orDash(t.URL))
		fmt.Fprintf(out, "  attribution:  %s\n", orDash(t.Attribution))
		if t.Note != "" {
			fmt.Fprintf(out, "  note:         %s\n", t.Note)
		}
	}
	for _, s := range sources.All() {
		print(fmt.Sprintf("%s  %s", s.Country(), s.Name()), s.Terms())
		if p, ok := s.(sources.Partial); ok {
			fmt.Fprintf(out, "  incomplete:   %s\n", p.Partial())
		}
	}
	print("BIC join  "+sources.BICMappingName, sources.BICMappingTerms)
	return nil
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}
