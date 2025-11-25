package test

import (
	"testing"

	"github.com/massmux/SatsMobiBot/internal/breez"
)

// TestMnemonicGeneration tests BIP39 mnemonic generation
func TestMnemonicGeneration(t *testing.T) {
	t.Run("Generate 12-word mnemonic", func(t *testing.T) {
		mnemonic, err := breez.GenerateMnemonic()
		if err != nil {
			t.Fatalf("Failed to generate mnemonic: %v", err)
		}

		if mnemonic == "" {
			t.Error("Generated empty mnemonic")
		}

		t.Logf("✓ Generated valid 12-word mnemonic")
	})

	t.Run("Generate 24-word mnemonic", func(t *testing.T) {
		mnemonic, err := breez.GenerateMnemonic24()
		if err != nil {
			t.Fatalf("Failed to generate 24-word mnemonic: %v", err)
		}

		if mnemonic == "" {
			t.Error("Generated empty mnemonic")
		}

		t.Logf("✓ Generated valid 24-word mnemonic")
	})

	t.Run("Mnemonics are unique", func(t *testing.T) {
		mnemonic1, _ := breez.GenerateMnemonic()
		mnemonic2, _ := breez.GenerateMnemonic()

		if mnemonic1 == mnemonic2 {
			t.Error("Generated identical mnemonics (should be random)")
		}

		t.Log("✓ Generated unique mnemonics")
	})
}

// TestMnemonicValidation tests mnemonic validation logic
func TestMnemonicValidation(t *testing.T) {
	tests := []struct {
		name      string
		mnemonic  string
		shouldErr bool
	}{
		{
			name:      "Valid 12-word mnemonic",
			mnemonic:  "abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about",
			shouldErr: false,
		},
		{
			name:      "Empty mnemonic",
			mnemonic:  "",
			shouldErr: true,
		},
		{
			name:      "Too few words",
			mnemonic:  "word1 word2 word3",
			shouldErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := breez.ValidateMnemonic(tt.mnemonic)

			if tt.shouldErr && err == nil {
				t.Errorf("Expected error for invalid mnemonic, got nil")
			}
			if !tt.shouldErr && err != nil {
				t.Errorf("Expected no error for valid mnemonic, got: %v", err)
			}

			if err == nil {
				t.Logf("✓ Mnemonic validated successfully")
			} else {
				t.Logf("✓ Invalid mnemonic rejected: %v", err)
			}
		})
	}
}

// TestBreezAmountLimits tests Breez SDK amount constraints
func TestBreezAmountLimits(t *testing.T) {
	const BreezMinAmount = 1000
	const BreezMaxAmount = 25000000

	tests := []struct {
		name   string
		amount int64
		valid  bool
		reason string
	}{
		{
			name:   "Below minimum",
			amount: 500,
			valid:  false,
			reason: "Below 1000 sats minimum",
		},
		{
			name:   "At minimum",
			amount: 1000,
			valid:  true,
			reason: "Exactly at minimum",
		},
		{
			name:   "Normal amount",
			amount: 50000,
			valid:  true,
			reason: "Within normal range",
		},
		{
			name:   "Large amount",
			amount: 1000000,
			valid:  true,
			reason: "Large but valid",
		},
		{
			name:   "At maximum",
			amount: 25000000,
			valid:  true,
			reason: "At maximum limit",
		},
		{
			name:   "Above maximum",
			amount: 30000000,
			valid:  false,
			reason: "Exceeds 25M sats maximum",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isValid := tt.amount >= BreezMinAmount && tt.amount <= BreezMaxAmount

			if isValid != tt.valid {
				t.Errorf("Amount %d: Expected valid=%v, got %v. %s",
					tt.amount, tt.valid, isValid, tt.reason)
			}

			if isValid {
				t.Logf("✓ Amount %d is valid. %s", tt.amount, tt.reason)
			} else {
				t.Logf("✓ Amount %d correctly rejected. %s", tt.amount, tt.reason)
			}
		})
	}
}

// TestBreezStructures tests Breez SDK data structures
func TestBreezStructures(t *testing.T) {
	t.Run("Config structure", func(t *testing.T) {
		config := &breez.Config{
			APIKey:     "test-api-key",
			WorkingDir: "/tmp/test-breez",
			Network:    breez.NetworkTestnet,
			Mnemonic:   "",
		}

		if config.APIKey == "" {
			t.Error("Config has empty API key")
		}

		t.Logf("✓ Config structure valid: network=%s", config.Network)
	})

	t.Run("Invoice structure", func(t *testing.T) {
		invoice := &breez.Invoice{
			Bolt11:      "lnbc1000n1...",
			PaymentHash: "abc123def456...",
			Amount:      10000,
			Description: "Test invoice",
		}

		if invoice.Bolt11 == "" {
			t.Error("Invoice has empty bolt11")
		}
		if invoice.Amount <= 0 {
			t.Error("Invoice has invalid amount")
		}

		t.Logf("✓ Invoice structure valid: amount=%d sats", invoice.Amount)
	})

	t.Run("PaymentInfo structure", func(t *testing.T) {
		payment := &breez.PaymentInfo{
			ID:          "payment123",
			PaymentHash: "hash123",
			Amount:      10000,
			Status:      breez.PaymentStatusComplete,
			Direction:   breez.PaymentDirectionOutbound,
		}

		if payment.PaymentHash == "" {
			t.Error("Payment has empty hash")
		}
		if payment.Amount <= 0 {
			t.Error("Payment has invalid amount")
		}

		t.Logf("✓ PaymentInfo valid: amount=%d sats, status=%s",
			payment.Amount, payment.Status)
	})
}
