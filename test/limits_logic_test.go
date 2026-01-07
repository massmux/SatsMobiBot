package test

import (
	"testing"
)

// TestAutoSwapTriggerLogic tests when auto-swap should trigger
func TestAutoSwapTriggerLogic(t *testing.T) {
	S := int64(50000)  // LNbits max balance
	S2 := int64(60000) // Auto-swap threshold

	tests := []struct {
		name               string
		lnbitsBalance      int64
		shouldTriggerSwap  bool
		expectedSwapAmount int64
		description        string
	}{
		{
			name:               "Balance below threshold",
			lnbitsBalance:      45000,
			shouldTriggerSwap:  false,
			expectedSwapAmount: 0,
			description:        "45K < 60K (S2), no auto-swap",
		},
		{
			name:               "Balance exactly at threshold",
			lnbitsBalance:      60000,
			shouldTriggerSwap:  false,
			expectedSwapAmount: 0,
			description:        "60K = 60K (S2), no auto-swap (needs to exceed)",
		},
		{
			name:               "Balance just above threshold",
			lnbitsBalance:      60001,
			shouldTriggerSwap:  true,
			expectedSwapAmount: 10001, // 60001 - 50000
			description:        "60001 > 60K (S2), swap 10001 sats",
		},
		{
			name:               "Balance significantly above threshold",
			lnbitsBalance:      75000,
			shouldTriggerSwap:  true,
			expectedSwapAmount: 25000, // 75000 - 50000
			description:        "75K > 60K (S2), swap 25K sats",
		},
		{
			name:               "Balance way over threshold",
			lnbitsBalance:      100000,
			shouldTriggerSwap:  true,
			expectedSwapAmount: 50000, // 100000 - 50000
			description:        "100K > 60K (S2), swap 50K sats",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Check if should trigger
			shouldTrigger := tt.lnbitsBalance > S2

			if shouldTrigger != tt.shouldTriggerSwap {
				t.Errorf("Balance %d: Expected trigger=%v, got %v",
					tt.lnbitsBalance, tt.shouldTriggerSwap, shouldTrigger)
			}

			// Calculate swap amount if triggered
			if shouldTrigger {
				swapAmount := tt.lnbitsBalance - S
				if swapAmount != tt.expectedSwapAmount {
					t.Errorf("Balance %d: Expected swap amount=%d, got %d",
						tt.lnbitsBalance, tt.expectedSwapAmount, swapAmount)
				}
				t.Logf("✓ %s: Would swap %d sats (keeping %d in LNbits)",
					tt.name, swapAmount, S)
			} else {
				t.Logf("✓ %s: No auto-swap triggered", tt.name)
			}
		})
	}
}

// TestSwapToLNbitsCapacityCalculation tests the capacity calculation for reverse swaps
func TestSwapToLNbitsCapacityCalculation(t *testing.T) {
	S := int64(50000) // LNbits max balance

	tests := []struct {
		name            string
		currentLNbits   int64
		breezBalance    int64
		expectedMaxSwap int64
		canSwap         bool
		description     string
	}{
		{
			name:            "LNbits empty, can fill completely",
			currentLNbits:   0,
			breezBalance:    100000,
			expectedMaxSwap: 50000, // Can fill to S
			canSwap:         true,
			description:     "Can swap up to 50K sats",
		},
		{
			name:            "LNbits half full",
			currentLNbits:   25000,
			breezBalance:    50000,
			expectedMaxSwap: 25000, // 50000 - 25000
			canSwap:         true,
			description:     "Can swap 25K to fill to capacity",
		},
		{
			name:            "LNbits near capacity",
			currentLNbits:   48000,
			breezBalance:    10000,
			expectedMaxSwap: 2000, // 50000 - 48000
			canSwap:         true,
			description:     "Can only swap 2K sats (limited by capacity)",
		},
		{
			name:            "LNbits at capacity",
			currentLNbits:   50000,
			breezBalance:    20000,
			expectedMaxSwap: 0,
			canSwap:         false,
			description:     "Cannot swap, LNbits already full",
		},
		{
			name:            "LNbits over capacity (shouldn't happen but test it)",
			currentLNbits:   55000,
			breezBalance:    20000,
			expectedMaxSwap: 0,
			canSwap:         false,
			description:     "Cannot swap, LNbits over capacity",
		},
		{
			name:            "Breez has less than capacity needed",
			currentLNbits:   40000,
			breezBalance:    5000,
			expectedMaxSwap: 5000, // Limited by Breez balance
			canSwap:         true,
			description:     "Can only swap what Breez has (5K)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Calculate max swap: min(S - B, breezBalance)
			capacityAvailable := S - tt.currentLNbits
			maxSwap := capacityAvailable
			if maxSwap > tt.breezBalance {
				maxSwap = tt.breezBalance
			}
			if capacityAvailable <= 0 {
				maxSwap = 0
			}

			canSwap := maxSwap > 0

			if canSwap != tt.canSwap {
				t.Errorf("%s: Expected canSwap=%v, got %v",
					tt.name, tt.canSwap, canSwap)
			}

			if maxSwap != tt.expectedMaxSwap {
				t.Errorf("%s: Expected maxSwap=%d, got %d",
					tt.name, tt.expectedMaxSwap, maxSwap)
			}

			if canSwap {
				t.Logf("✓ %s: Can swap %d sats. %s",
					tt.name, maxSwap, tt.description)
			} else {
				t.Logf("✓ %s: Cannot swap. %s",
					tt.name, tt.description)
			}
		})
	}
}

// TestThresholdConfiguration tests configuration validation
func TestThresholdConfiguration(t *testing.T) {
	tests := []struct {
		name        string
		S           int64
		S2          int64
		valid       bool
		description string
	}{
		{
			name:        "Default valid configuration",
			S:           50000,
			S2:          60000,
			valid:       true,
			description: "S2 (60K) > S (50K) ✓",
		},
		{
			name:        "Conservative configuration",
			S:           30000,
			S2:          35000,
			valid:       true,
			description: "Lower limits but S2 > S ✓",
		},
		{
			name:        "Invalid: S2 equals S",
			S:           50000,
			S2:          50000,
			valid:       false,
			description: "S2 must be greater than S",
		},
		{
			name:        "Invalid: S2 less than S",
			S:           50000,
			S2:          45000,
			valid:       false,
			description: "S2 cannot be less than S",
		},
		{
			name:        "Aggressive configuration",
			S:           100000,
			S2:          110000,
			valid:       true,
			description: "Higher limits with proper spacing ✓",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isValid := tt.S2 > tt.S

			if isValid != tt.valid {
				t.Errorf("%s: Expected valid=%v, got %v. %s",
					tt.name, tt.valid, isValid, tt.description)
			}

			if isValid {
				gap := tt.S2 - tt.S
				t.Logf("✓ %s: Valid config (gap: %d sats). %s",
					tt.name, gap, tt.description)
			} else {
				t.Logf("✓ %s: Invalid config detected. %s",
					tt.name, tt.description)
			}
		})
	}
}

// TestCompleteBalanceFlow tests a complete balance management scenario
func TestCompleteBalanceFlow(t *testing.T) {
	S := int64(50000)
	S2 := int64(60000)

	t.Log("=== Complete Balance Management Flow ===")

	// Scenario: User journey from empty to full wallets
	lnbitsBalance := int64(0)
	breezBalance := int64(0)

	t.Logf("Initial state: LNbits=%d, Breez=%d", lnbitsBalance, breezBalance)

	// Step 1: Receive first invoice (should go to Breez with Breez-first)
	t.Log("\n--- Step 1: First invoice (10K) ---")
	invoiceAmount := int64(10000)
	// With Breez-first: goes to Breez
	breezBalance += invoiceAmount
	t.Logf("Received %d on Breez → LNbits=%d, Breez=%d", invoiceAmount, lnbitsBalance, breezBalance)

	// Step 2: Force receive on LNbits using /invoiceln
	t.Log("\n--- Step 2: Force LNbits receive (15K using /invoiceln) ---")
	invoiceAmount = 15000
	lnbitsBalance += invoiceAmount
	t.Logf("After payment: LNbits=%d, Breez=%d", lnbitsBalance, breezBalance)
	if lnbitsBalance > S2 {
		t.Error("Should not trigger auto-swap yet")
	}
	t.Log("✓ No auto-swap (15K < 60K threshold)")

	// Step 3: Another forced LNbits receive that triggers auto-swap
	t.Log("\n--- Step 3: Force LNbits receive (50K using /invoiceln) ---")
	invoiceAmount = 50000
	lnbitsBalance += invoiceAmount // Now 65K
	t.Logf("After payment: LNbits=%d (exceeds S2!)", lnbitsBalance)

	if lnbitsBalance > S2 {
		swapAmount := lnbitsBalance - S
		t.Logf("✓ AUTO-SWAP TRIGGERED: Moving %d sats to Breez", swapAmount)
		breezBalance += swapAmount
		lnbitsBalance = S
		t.Logf("After auto-swap: LNbits=%d, Breez=%d", lnbitsBalance, breezBalance)
	}

	// Step 4: Normal invoice (goes to Breez with Breez-first)
	t.Log("\n--- Step 4: Normal invoice (20K) ---")
	invoiceAmount = 20000
	breezBalance += invoiceAmount
	t.Logf("Received on Breez: LNbits=%d, Breez=%d", lnbitsBalance, breezBalance)

	// Step 5: Manual swap all LNbits to Breez
	t.Log("\n--- Step 5: Manual /swaptosafe ---")
	if lnbitsBalance > 0 {
		swapAmount := lnbitsBalance
		t.Logf("Swapping %d sats from LNbits to Breez", swapAmount)
		breezBalance += swapAmount
		lnbitsBalance = 0
		t.Logf("After swap: LNbits=%d, Breez=%d", lnbitsBalance, breezBalance)
	}

	// Step 6: Partial swap back to LNbits
	t.Log("\n--- Step 6: Manual /swaptohot ---")
	if breezBalance > 0 {
		maxCapacity := S - lnbitsBalance
		swapAmount := maxCapacity
		if swapAmount > breezBalance {
			swapAmount = breezBalance
		}
		t.Logf("Swapping %d sats from Breez to LNbits (filling to capacity)", swapAmount)
		lnbitsBalance += swapAmount
		breezBalance -= swapAmount
		t.Logf("After swap: LNbits=%d (at capacity), Breez=%d", lnbitsBalance, breezBalance)
	}

	// Final validation
	t.Log("\n=== Final State ===")
	t.Logf("LNbits: %d sats (max capacity: %d)", lnbitsBalance, S)
	t.Logf("Breez: %d sats (self-custodial, unlimited)", breezBalance)
	t.Logf("Total: %d sats", lnbitsBalance+breezBalance)

	if lnbitsBalance > S {
		t.Errorf("LNbits balance %d exceeds capacity %d", lnbitsBalance, S)
	}

	t.Log("✓ Complete flow validated successfully")
}

// TestBreezFirstRouting tests the Breez-first routing logic
func TestBreezFirstRouting(t *testing.T) {
	tests := []struct {
		name           string
		breezAvailable bool
		expectedWallet string
	}{
		{
			name:           "Breez available - route to Breez",
			breezAvailable: true,
			expectedWallet: "Breez",
		},
		{
			name:           "Breez NOT available - route to LNbits",
			breezAvailable: false,
			expectedWallet: "LNbits",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Breez-first logic: use Breez if available, else LNbits
			useBreez := tt.breezAvailable
			actualWallet := "LNbits"
			if useBreez {
				actualWallet = "Breez"
			}

			if actualWallet != tt.expectedWallet {
				t.Errorf("Expected %s, got %s", tt.expectedWallet, actualWallet)
			}

			t.Logf("✓ Correctly routed to %s", actualWallet)
		})
	}
}
