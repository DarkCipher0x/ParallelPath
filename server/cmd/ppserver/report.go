package main

import (
	"errors"
	"fmt"
	"os"

	"pp/engine/app"
)

// cmdVerifyReport checks a court-ready report's signature and signer (anyone can run it; it needs no server data).
func cmdVerifyReport(args []string) error {
	if len(args) != 1 {
		return errors.New("usage: ppserver verify-report FILE.json")
	}
	b, err := os.ReadFile(args[0])
	if err != nil {
		return err
	}
	c := app.VerifyReportJSON(b)
	if !c.Valid {
		return errors.New("NOT VALID: " + c.Problem)
	}
	fmt.Println("Valid ParallelPath report: the signature matches, so nothing has been changed since it was signed.")
	fmt.Printf("  Report ID      %s\n  Family         %s\n  Period         %s to %s\n  Signed by      %s\n", c.ID, c.Family, c.From, c.To, c.ByName)
	fmt.Printf("  Signer ID      %s  (the person's identity-key fingerprint)\n  Made at        %s\n", c.Signer, c.Generated.Format("2006-01-02 15:04:05 MST"))
	fmt.Printf("  Audit head     entry #%d  %s\n  SHA3-512       %s\n", c.AuditSeq, c.AuditHash, c.SHA3)
	fmt.Println("To check the audit head against the family's chain, open the report in ParallelPath (Audit log > Verify a report).")
	return nil
}
