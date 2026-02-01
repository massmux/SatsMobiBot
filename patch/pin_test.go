package breez_test

import (
	"testing"
	"time"

	"github.com/massmux/SatsMobiBot/internal/breez"
	"github.com/massmux/SatsMobiBot/internal/lnbits"
)

// TestPinValidation testa la validazione dei PIN
func TestPinValidation(t *testing.T) {
	tests := []struct {
		name    string
		pin     string
		wantErr bool
	}{
		{"Valid 4 digits", "1234", false},
		{"Valid 5 digits", "12345", false},
		{"Valid 6 digits", "123456", false},
		{"Too short", "123", true},
		{"Too long", "1234567", true},
		{"Contains letters", "12ab", true},
		{"Contains spaces", "12 34", true},
		{"All same digit", "1111", true},
		{"All same digit 5", "11111", true},
		{"All same digit 6", "111111", true},
		{"Sequential ascending", "1234", true},
		{"Sequential ascending 5", "12345", true},
		{"Sequential ascending 6", "123456", true},
		{"Sequential descending", "4321", true},
		{"Valid complex", "2468", false},
		{"Valid mixed", "1928", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := breez.ValidatePin(tt.pin)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidatePin(%q) error = %v, wantErr %v", tt.pin, err, tt.wantErr)
			}
		})
	}
}

// TestGenerateSalt testa la generazione del salt
func TestGenerateSalt(t *testing.T) {
	salt1, err := breez.GenerateSalt()
	if err != nil {
		t.Fatalf("GenerateSalt failed: %v", err)
	}

	// Verifica che sia una stringa hex valida di 64 caratteri (32 bytes)
	if len(salt1) != 64 {
		t.Errorf("Expected salt length 64, got %d", len(salt1))
	}

	// Genera un secondo salt e verifica che sia diverso
	salt2, err := breez.GenerateSalt()
	if err != nil {
		t.Fatalf("GenerateSalt failed: %v", err)
	}

	if salt1 == salt2 {
		t.Error("Two generated salts should be different")
	}
}

// TestHashAndVerifyPin testa l'hashing e la verifica del PIN
func TestHashAndVerifyPin(t *testing.T) {
	pin := "1234"

	// Hash PIN
	hash, err := breez.HashPin(pin)
	if err != nil {
		t.Fatalf("HashPin failed: %v", err)
	}

	// Verifica che l'hash non sia vuoto
	if hash == "" {
		t.Error("Hash should not be empty")
	}

	// Verifica che l'hash non sia uguale al PIN
	if hash == pin {
		t.Error("Hash should not equal plain PIN")
	}

	// Verifica con PIN corretto
	err = breez.VerifyPin(pin, hash)
	if err != nil {
		t.Errorf("VerifyPin with correct PIN failed: %v", err)
	}

	// Verifica con PIN errato
	err = breez.VerifyPin("5678", hash)
	if err == nil {
		t.Error("Expected error when verifying wrong PIN, got nil")
	}
}

// TestDeriveKeyFromPin testa la derivazione della chiave dal PIN
func TestDeriveKeyFromPin(t *testing.T) {
	pin := "1234"
	salt, _ := breez.GenerateSalt()

	// Deriva chiave
	key1, err := breez.DeriveKeyFromPin(pin, salt)
	if err != nil {
		t.Fatalf("DeriveKeyFromPin failed: %v", err)
	}

	// Verifica lunghezza chiave (32 bytes = 64 hex chars)
	if len(key1) != 64 {
		t.Errorf("Expected key length 64, got %d", len(key1))
	}

	// Deriva di nuovo con stesso PIN e salt - dovrebbe dare stessa chiave
	key2, err := breez.DeriveKeyFromPin(pin, salt)
	if err != nil {
		t.Fatalf("DeriveKeyFromPin failed: %v", err)
	}

	if key1 != key2 {
		t.Error("Same PIN and salt should derive same key")
	}

	// Deriva con PIN diverso - dovrebbe dare chiave diversa
	key3, err := breez.DeriveKeyFromPin("5678", salt)
	if err != nil {
		t.Fatalf("DeriveKeyFromPin failed: %v", err)
	}

	if key1 == key3 {
		t.Error("Different PINs should derive different keys")
	}

	// Deriva con salt diverso - dovrebbe dare chiave diversa
	salt2, _ := breez.GenerateSalt()
	key4, err := breez.DeriveKeyFromPin(pin, salt2)
	if err != nil {
		t.Fatalf("DeriveKeyFromPin failed: %v", err)
	}

	if key1 == key4 {
		t.Error("Different salts should derive different keys")
	}
}

// TestEncryptDecryptWithPin testa cifratura e decifratura con PIN
func TestEncryptDecryptWithPin(t *testing.T) {
	// Test mnemonic
	mnemonic := "abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about"
	pin := "1234"

	// Genera salt
	salt, err := breez.GenerateSalt()
	if err != nil {
		t.Fatalf("GenerateSalt failed: %v", err)
	}

	// Cifra
	encrypted, err := breez.EncryptMnemonicWithPin(mnemonic, pin, salt)
	if err != nil {
		t.Fatalf("EncryptMnemonicWithPin failed: %v", err)
	}

	// Verifica che sia cifrato (diverso dall'originale)
	if encrypted == mnemonic {
		t.Error("Encrypted should be different from plaintext")
	}

	// Decifra
	decrypted, err := breez.DecryptMnemonicWithPin(encrypted, pin, salt)
	if err != nil {
		t.Fatalf("DecryptMnemonicWithPin failed: %v", err)
	}

	// Verifica che corrisponda all'originale
	if decrypted != mnemonic {
		t.Errorf("Decrypted mnemonic doesn't match original.\nGot: %s\nWant: %s", decrypted, mnemonic)
	}
}

// TestWrongPinFails testa che un PIN errato non possa decifrare
func TestWrongPinFails(t *testing.T) {
	mnemonic := "abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about"
	correctPin := "1234"
	wrongPin := "5678"

	salt, _ := breez.GenerateSalt()
	encrypted, _ := breez.EncryptMnemonicWithPin(mnemonic, correctPin, salt)

	// Prova a decifrare con PIN errato
	_, err := breez.DecryptMnemonicWithPin(encrypted, wrongPin, salt)
	if err == nil {
		t.Error("Expected error when decrypting with wrong PIN, got nil")
	}
}

// TestChangePinRencrypt testa il cambio PIN
func TestChangePinRencrypt(t *testing.T) {
	mnemonic := "abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about"
	oldPin := "1234"
	newPin := "5678"

	// Setup
	oldSalt, _ := breez.GenerateSalt()
	newSalt, _ := breez.GenerateSalt()

	// Cifra con vecchio PIN
	encrypted, err := breez.EncryptMnemonicWithPin(mnemonic, oldPin, oldSalt)
	if err != nil {
		t.Fatalf("Initial encryption failed: %v", err)
	}

	// Ricifra con nuovo PIN
	reencrypted, err := breez.ChangePinRencrypt(encrypted, oldPin, oldSalt, newPin, newSalt)
	if err != nil {
		t.Fatalf("ChangePinRencrypt failed: %v", err)
	}

	// Verifica che possa decifrare con nuovo PIN
	decrypted, err := breez.DecryptMnemonicWithPin(reencrypted, newPin, newSalt)
	if err != nil {
		t.Fatalf("Decryption with new PIN failed: %v", err)
	}

	if decrypted != mnemonic {
		t.Errorf("Re-encrypted mnemonic doesn't match original")
	}

	// Verifica che NON possa decifrare con vecchio PIN
	_, err = breez.DecryptMnemonicWithPin(reencrypted, oldPin, oldSalt)
	if err == nil {
		t.Error("Should not be able to decrypt with old PIN after re-encryption")
	}
}

// TestUserHasPin testa il metodo HasPin della struct User
func TestUserHasPin(t *testing.T) {
	user := &lnbits.User{}

	// User senza PIN
	if user.HasPin() {
		t.Error("User without PIN returned HasPin() = true")
	}

	// User con solo hash (incompleto)
	user.PinHash = "somehash"
	if user.HasPin() {
		t.Error("User with only hash returned HasPin() = true")
	}

	// User con solo salt (incompleto)
	user.PinHash = ""
	user.PinSalt = "somesalt"
	if user.HasPin() {
		t.Error("User with only salt returned HasPin() = true")
	}

	// User con entrambi (completo)
	user.PinHash = "somehash"
	user.PinSalt = "somesalt"
	if !user.HasPin() {
		t.Error("User with hash and salt returned HasPin() = false")
	}
}

// TestUserPinLocked testa il metodo IsPinLocked della struct User
func TestUserPinLocked(t *testing.T) {
	user := &lnbits.User{}

	// Non bloccato
	if user.IsPinLocked() {
		t.Error("User without lockout returned IsPinLocked() = true")
	}

	// Bloccato nel futuro
	future := time.Now().Add(10 * time.Minute)
	user.PinLockedUntil = &future
	if !user.IsPinLocked() {
		t.Error("User with future lockout returned IsPinLocked() = false")
	}

	// Lockout scaduto
	past := time.Now().Add(-10 * time.Minute)
	user.PinLockedUntil = &past
	if user.IsPinLocked() {
		t.Error("User with expired lockout returned IsPinLocked() = true")
	}
}

// TestMultipleEncryptionsAreDifferent testa che cifrature multiple diano risultati diversi
func TestMultipleEncryptionsAreDifferent(t *testing.T) {
	mnemonic := "abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about"
	pin := "1234"
	salt, _ := breez.GenerateSalt()

	// Cifra due volte la stessa mnemonic
	encrypted1, _ := breez.EncryptMnemonicWithPin(mnemonic, pin, salt)
	encrypted2, _ := breez.EncryptMnemonicWithPin(mnemonic, pin, salt)

	// Le cifrature dovrebbero essere diverse (grazie al nonce random in GCM)
	if encrypted1 == encrypted2 {
		t.Error("Multiple encryptions of same data should produce different ciphertexts")
	}

	// Ma entrambe dovrebbero decifrare correttamente
	decrypted1, err1 := breez.DecryptMnemonicWithPin(encrypted1, pin, salt)
	decrypted2, err2 := breez.DecryptMnemonicWithPin(encrypted2, pin, salt)

	if err1 != nil || err2 != nil {
		t.Fatalf("Decryption failed: %v, %v", err1, err2)
	}

	if decrypted1 != mnemonic || decrypted2 != mnemonic {
		t.Error("Both encryptions should decrypt to original mnemonic")
	}
}

// Benchmark tests

func BenchmarkValidatePin(b *testing.B) {
	pin := "1234"
	for i := 0; i < b.N; i++ {
		breez.ValidatePin(pin)
	}
}

func BenchmarkGenerateSalt(b *testing.B) {
	for i := 0; i < b.N; i++ {
		breez.GenerateSalt()
	}
}

func BenchmarkHashPin(b *testing.B) {
	pin := "1234"
	for i := 0; i < b.N; i++ {
		breez.HashPin(pin)
	}
}

func BenchmarkDeriveKeyFromPin(b *testing.B) {
	pin := "1234"
	salt, _ := breez.GenerateSalt()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		breez.DeriveKeyFromPin(pin, salt)
	}
}

func BenchmarkEncryptMnemonicWithPin(b *testing.B) {
	mnemonic := "abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about"
	pin := "1234"
	salt, _ := breez.GenerateSalt()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		breez.EncryptMnemonicWithPin(mnemonic, pin, salt)
	}
}

func BenchmarkDecryptMnemonicWithPin(b *testing.B) {
	mnemonic := "abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about"
	pin := "1234"
	salt, _ := breez.GenerateSalt()
	encrypted, _ := breez.EncryptMnemonicWithPin(mnemonic, pin, salt)
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		breez.DecryptMnemonicWithPin(encrypted, pin, salt)
	}
}
