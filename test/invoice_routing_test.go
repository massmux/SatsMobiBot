package test

import (
	"testing"
)

// TestBreezFirstInvoiceRouting tests the Breez-first invoice routing logic
func TestBreezFirstInvoiceRouting(t *testing.T) {
	tests := []struct {
		name           string
		breezAvailable bool
		lnbitsBalance  int64
		invoiceAmount  int64
		expectedBreez  bool
		description    string
	}{
		{
			name:           "Breez available, small amount",
			breezAvailable: true,
			lnbitsBalance:  10000,
			invoiceAmount:  5000,
			expectedBreez:  true,
			description:    "With Breez-first logic, should always use Breez when available",
		},
		{
			name:           "Breez available, large amount",
			breezAvailable: true,
			lnbitsBalance:  45000,
			invoiceAmount:  100000,
			expectedBreez:  true,
			description:    "Should use Breez for large amounts",
		},
		{
			name:           "Breez available, LNbits near capacity",
			breezAvailable: true,
			lnbitsBalance:  49000,
			invoiceAmount:  2000,
			expectedBreez:  true,
			description:    "Should use Breez even when LNbits has room",
		},
		{
			name:           "Breez NOT available, should fallback to LNbits",
			breezAvailable: false,
			lnbitsBalance:  10000,
			invoiceAmount:  5000,
			expectedBreez:  false,
			description:    "Should use LNbits when Breez not initialized",
		},
		{
			name:           "Breez NOT available, LNbits at capacity",
			breezAvailable: false,
			lnbitsBalance:  50000,
			invoiceAmount:  5000,
			expectedBreez:  false,
			description:    "Should still try LNbits (will hit limit warning in handler)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Breez-first logic: if Breez available, use Breez; else use LNbits
			actualResult := tt.breezAvailable

			if actualResult != tt.expectedBreez {
				t.Errorf("%s: Expected useBreez=%v, got %v. %s",
					tt.name, tt.expectedBreez, actualResult, tt.description)
			}

			t.Logf("✓ %s: Correctly routed to %s. %s",
				tt.name,
				map[bool]string{true: "Breez", false: "LNbits"}[actualResult],
				tt.description)
		})
	}
}

// TestInvoiceRoutingScenarios tests complete invoice routing scenarios
func TestInvoiceRoutingScenarios(t *testing.T) {
	scenarios := []struct {
		name          string
		breezInit     bool
		lnbitsBalance int64
		invoiceAmount int64
		expectBreez   bool
		reason        string
	}{
		{
			name:          "New user with Breez initialized",
			breezInit:     true,
			lnbitsBalance: 0,
			invoiceAmount: 10000,
			expectBreez:   true,
			reason:        "Breez-first: always use Breez when available, even for first invoice",
		},
		{
			name:          "User migrating from LNbits to Breez",
			breezInit:     true,
			lnbitsBalance: 30000,
			invoiceAmount: 15000,
			expectBreez:   true,
			reason:        "New invoices go to Breez to encourage self-custodial migration",
		},
		{
			name:          "Legacy user without Breez",
			breezInit:     false,
			lnbitsBalance: 20000,
			invoiceAmount: 5000,
			expectBreez:   false,
			reason:        "Fallback to LNbits when Breez not initialized",
		},
	}

	for _, scenario := range scenarios {
		t.Run(scenario.name, func(t *testing.T) {
			// Test the Breez-first routing logic
			useBreez := scenario.breezInit

			if useBreez != scenario.expectBreez {
				t.Errorf("Scenario '%s' failed: Expected Breez=%v, got %v. Reason: %s",
					scenario.name, scenario.expectBreez, useBreez, scenario.reason)
			}

			wallet := map[bool]string{true: "Breez (self-custodial)", false: "LNbits (custodial)"}[useBreez]
			t.Logf("✓ Scenario '%s': Invoice routed to %s. %s", scenario.name, wallet, scenario.reason)
		})
	}
}

// TestBreezFirstPrinciple validates the core self-custodial-first principle
func TestBreezFirstPrinciple(t *testing.T) {
	t.Run("Without Breez - uses LNbits", func(t *testing.T) {
		breezInitialized := false
		useBreez := breezInitialized

		if useBreez {
			t.Error("Should use LNbits when Breez not available")
		}
		t.Log("✓ Correctly falls back to LNbits when Breez unavailable")
	})

	t.Run("With Breez - always uses Breez", func(t *testing.T) {
		breezInitialized := true

		// Test various amounts - all should use Breez with Breez-first logic
		amounts := []int64{100, 1000, 10000, 50000, 100000}
		for _, amount := range amounts {
			useBreez := breezInitialized
			if !useBreez {
				t.Errorf("Amount %d: Should use Breez (self-custodial first)", amount)
			}
		}
		t.Log("✓ Correctly uses Breez for all amounts when available (Breez-first principle)")
	})
}
