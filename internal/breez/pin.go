package breez

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"

	log "github.com/sirupsen/logrus"
	"golang.org/x/crypto/bcrypt"
	"golang.org/x/crypto/pbkdf2"
)

const (
	// PBKDF2 parameters
	PBKDF2Iterations = 100000
	KeyLength        = 32 // 256 bits for AES-256
	SaltLength       = 32 // 256 bits

	// PIN constraints
	MinPinLength = 4
	MaxPinLength = 6
)

// PinError rappresenta un errore relativo al PIN
type PinError struct {
	Code    string
	Message string
}

func (e *PinError) Error() string {
	return e.Message
}

// ValidatePin valida un PIN
func ValidatePin(pin string) error {
	// Rimuovi spazi
	pin = regexp.MustCompile(`\s+`).ReplaceAllString(pin, "")

	// Verifica lunghezza
	if len(pin) < MinPinLength || len(pin) > MaxPinLength {
		return &PinError{
			Code:    "INVALID_LENGTH",
			Message: fmt.Sprintf("PIN must be %d-%d digits", MinPinLength, MaxPinLength),
		}
	}

	// Verifica che sia solo numerico
	if !regexp.MustCompile(`^\d+$`).MatchString(pin) {
		return &PinError{
			Code:    "NOT_NUMERIC",
			Message: "PIN must contain only digits",
		}
	}

	// Verifica che non sia troppo semplice (tutti stessi digit)
	allSame := true
	if len(pin) > 0 {
		firstChar := pin[0]
		for i := 1; i < len(pin); i++ {
			if pin[i] != firstChar {
				allSame = false
				break
			}
		}
	}
	if allSame {
		return &PinError{
			Code:    "TOO_SIMPLE",
			Message: "PIN cannot be all the same digit (e.g. 1111)",
		}
	}

	// Verifica che non sia sequenziale
	if isSequential(pin) {
		return &PinError{
			Code:    "SEQUENTIAL",
			Message: "PIN cannot be sequential (e.g. 1234)",
		}
	}

	return nil
}

// isSequential verifica se il PIN è una sequenza
func isSequential(pin string) bool {
	sequential := []string{"0123", "1234", "2345", "3456", "4567", "5678", "6789",
		"3210", "4321", "5432", "6543", "7654", "8765", "9876"}
	for _, seq := range sequential {
		if pin == seq || pin == seq+"5" || pin == seq+"6" {
			return true
		}
	}
	return false
}

// GenerateSalt genera un salt casuale per PBKDF2
func GenerateSalt() (string, error) {
	salt := make([]byte, SaltLength)
	_, err := rand.Read(salt)
	if err != nil {
		return "", fmt.Errorf("failed to generate salt: %w", err)
	}
	return hex.EncodeToString(salt), nil
}

// HashPin crea un hash bcrypt del PIN per la verifica
func HashPin(pin string) (string, error) {
	// Usa costo 12 per bcrypt (buon bilanciamento sicurezza/performance)
	hashedBytes, err := bcrypt.GenerateFromPassword([]byte(pin), 12)
	if err != nil {
		return "", fmt.Errorf("failed to hash PIN: %w", err)
	}
	return string(hashedBytes), nil
}

// VerifyPin verifica che un PIN corrisponda all'hash
func VerifyPin(pin, hashedPin string) error {
	err := bcrypt.CompareHashAndPassword([]byte(hashedPin), []byte(pin))
	if err != nil {
		if err == bcrypt.ErrMismatchedHashAndPassword {
			return &PinError{
				Code:    "INCORRECT_PIN",
				Message: "Incorrect PIN",
			}
		}
		return fmt.Errorf("failed to verify PIN: %w", err)
	}
	return nil
}

// DeriveKeyFromPin deriva una chiave di cifratura dal PIN usando PBKDF2
func DeriveKeyFromPin(pin, saltHex string) (string, error) {
	// Decodifica il salt da hex
	salt, err := hex.DecodeString(saltHex)
	if err != nil {
		return "", fmt.Errorf("failed to decode salt: %w", err)
	}

	// Deriva la chiave usando PBKDF2 con SHA-256
	key := pbkdf2.Key([]byte(pin), salt, PBKDF2Iterations, KeyLength, sha256.New)

	log.Debugf("[PIN] Derived %d-byte key from PIN using PBKDF2 (%d iterations)", len(key), PBKDF2Iterations)

	return hex.EncodeToString(key), nil
}

// EncryptMnemonicWithPin cifra una mnemonic usando un PIN
// Questa è una funzione di convenienza che combina derivazione chiave + cifratura
func EncryptMnemonicWithPin(mnemonic, pin, saltHex string) (string, error) {
	// Deriva la chiave dal PIN
	keyHex, err := DeriveKeyFromPin(pin, saltHex)
	if err != nil {
		return "", fmt.Errorf("failed to derive key from PIN: %w", err)
	}

	// Cifra la mnemonic con la chiave derivata
	encrypted, err := EncryptMnemonic(mnemonic, keyHex)
	if err != nil {
		return "", fmt.Errorf("failed to encrypt mnemonic: %w", err)
	}

	log.Debug("[PIN] Mnemonic encrypted with PIN-derived key")
	return encrypted, nil
}

// DecryptMnemonicWithPin decifra una mnemonic usando un PIN
// Questa è una funzione di convenienza che combina derivazione chiave + decifratura
func DecryptMnemonicWithPin(encryptedMnemonic, pin, saltHex string) (string, error) {
	// Deriva la chiave dal PIN
	keyHex, err := DeriveKeyFromPin(pin, saltHex)
	if err != nil {
		return "", fmt.Errorf("failed to derive key from PIN: %w", err)
	}

	// Decifra la mnemonic con la chiave derivata
	mnemonic, err := DecryptMnemonic(encryptedMnemonic, keyHex)
	if err != nil {
		return "", fmt.Errorf("failed to decrypt mnemonic: %w", err)
	}

	log.Debug("[PIN] Mnemonic decrypted with PIN-derived key")
	return mnemonic, nil
}

// ChangePinRencrypt ricifra una mnemonic con un nuovo PIN
func ChangePinRencrypt(encryptedMnemonic, oldPin, oldSalt, newPin, newSalt string) (string, error) {
	// Decifra con il vecchio PIN
	mnemonic, err := DecryptMnemonicWithPin(encryptedMnemonic, oldPin, oldSalt)
	if err != nil {
		return "", fmt.Errorf("failed to decrypt with old PIN: %w", err)
	}

	// Ricifra con il nuovo PIN
	newEncrypted, err := EncryptMnemonicWithPin(mnemonic, newPin, newSalt)
	if err != nil {
		return "", fmt.Errorf("failed to encrypt with new PIN: %w", err)
	}

	log.Info("[PIN] Mnemonic re-encrypted with new PIN")
	return newEncrypted, nil
}
