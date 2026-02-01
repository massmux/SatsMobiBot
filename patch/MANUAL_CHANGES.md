# Modifiche Manuali Necessarie

Questi file non possono essere sostituiti completamente perché dipendono dalla tua implementazione specifica. Qui trovi le modifiche da applicare manualmente.

---

## 1. File: `internal/telegram/handler.go`

### Modifica A: Aggiungi comando /setpin

Cerca la funzione `OnCommand()` e aggiungi questa riga:

```go
func (bot *TipBot) OnCommand() {
	bot.Telegram.Handle("/start", bot.anyTextHandler(bot.startHandler))
	bot.Telegram.Handle("/balance", bot.anyTextHandler(bot.balanceHandler))
	bot.Telegram.Handle("/send", bot.anyTextHandler(bot.sendHandler))
	// ... altri comandi esistenti ...
	
	// ✨ AGGIUNGI QUESTA RIGA:
	bot.Telegram.Handle("/setpin", bot.anyTextHandler(bot.setPinHandler))
	
	// ... resto dei comandi ...
}
```

### Modifica B: Gestisci stati PIN

Cerca dove gestisci gli stati utente (potrebbe essere in un file separato come `state.go` o nella funzione che processa i messaggi). Aggiungi questi casi:

```go
func (bot *TipBot) handleMessageWithState(ctx intercept.Context) (intercept.Context, error) {
	user := LoadUser(ctx)
	
	switch user.StateKey {
	case lnbits.UserStateConfirmPayment:
		// ... existing code ...
	
	case lnbits.UserStateConfirmSend:
		// ... existing code ...
	
	// ✨ AGGIUNGI QUESTI CASI:
	case lnbits.UserStateEnterNewPin,
		lnbits.UserStateConfirmNewPin,
		lnbits.UserStateEnterPinForOperation,
		lnbits.UserStateEnterPinForBackup,
		lnbits.UserStateEnterPinForPayment:
		return bot.handlePinInput(ctx)
	
	// ... altri casi ...
	}
	
	return ctx, nil
}
```

**NOTA**: Se non hai una funzione `handleMessageWithState`, cerca dove processi i messaggi in base allo stato dell'utente e aggiungi lì la gestione degli stati PIN.

---

## 2. File: `internal/telegram/start.go`

### Modifica A: Aggiungi controllo PIN in initUserBreezWallet

Cerca la funzione `initUserBreezWallet` (circa linea 156) e **all'inizio della funzione** aggiungi questo controllo:

```go
func (bot *TipBot) initUserBreezWallet(user *lnbits.User) error {
	var mnemonic string
	var err error

	// ✨ AGGIUNGI QUESTO BLOCCO ALL'INIZIO:
	// Check if user has PIN set
	if !user.HasPin() {
		log.Infof("[Breez] User %s needs to set PIN first", GetUserStr(user.Telegram))
		
		msg := "🔐 *Welcome to Your Self-Custodial Wallet*\n\n" +
			"To protect your funds, you need to set a PIN.\n\n" +
			"This PIN will:\n" +
			"• Encrypt your seed phrase\n" +
			"• Protect your wallet access\n" +
			"• Cannot be recovered if forgotten\n\n" +
			"Use /setpin to continue."
		
		bot.trySendMessage(user.Telegram, msg, tb.ModeMarkdown)
		return fmt.Errorf("PIN required - user must set PIN first")
	}

	// Check if user has encrypted mnemonic stored in DB
	if user.BreezMnemonic != "" {
		// ... existing code continua normalmente ...
```

### Modifica B: Gestisci creazione wallet con PIN

Nella stessa funzione `initUserBreezWallet`, cerca il blocco dove genera una nuova mnemonic per utenti nuovi. Dovrebbe essere circa qui:

```go
	} else {
		// Generate new mnemonic for first-time user
		log.Infof("[Breez] Generating new mnemonic for user %s", GetUserStr(user.Telegram))
```

**SOSTITUISCI** quel blocco con:

```go
	} else {
		// ✨ NUOVO CODICE: Generate new mnemonic with PIN protection
		log.Infof("[Breez] Generating new mnemonic for user %s", GetUserStr(user.Telegram))
		
		// Request PIN to encrypt the new mnemonic
		bot.trySendMessage(user.Telegram, 
			"🔐 Enter your PIN to create your secure wallet:")
		
		user.StateKey = lnbits.UserStateEnterPinForOperation
		user.StateData = "create_wallet"
		UpdateUserRecord(user, *bot)
		
		return fmt.Errorf("waiting for PIN to create wallet")
	}
```

---

## 3. File: `internal/database/migrations.go`

### Modifica: Aggiungi chiamata alla migration PIN

Cerca la funzione principale delle migration (potrebbe chiamarsi `RunMigrations`, `Migrate`, o simile). Aggiungi la chiamata alla nuova migration:

```go
func RunMigrations(db *gorm.DB) error {
	log.Info("[Migration] Starting database migrations")
	
	// ... existing migrations ...
	
	// ✨ AGGIUNGI QUESTA CHIAMATA:
	if err := MigratePinFields(db); err != nil {
		log.Errorf("[Migration] Failed to run PIN migration: %s", err)
		return fmt.Errorf("PIN migration failed: %w", err)
	}
	
	// ... rest of migrations ...
	
	log.Info("[Migration] All migrations completed successfully")
	return nil
}
```

---

## 4. File: `go.mod`

Assicurati di avere queste dipendenze (se non ci sono, esegui `go get`):

```
go get golang.org/x/crypto/pbkdf2
go get golang.org/x/crypto/bcrypt
```

Poi esegui:
```bash
go mod tidy
```

---

## 5. File: `translations/en.toml` (Opzionale ma Consigliato)

Aggiungi questi messaggi alla fine del file:

```toml
# PIN messages
pinSetTitle = "🔐 Set Your PIN"
pinSetMessage = """
Please choose a PIN to secure your wallet.

Requirements:
• 4-6 digits
• Not all same digit (e.g. 1111)
• Not sequential (e.g. 1234)

Enter your PIN:"""

pinConfirmMessage = "✅ PIN accepted!\n\nPlease confirm your PIN by entering it again:"
pinMismatchMessage = "❌ PINs don't match!\n\nLet's try again. Enter your new PIN:"

pinSetSuccessMessage = """✅ **PIN Set Successfully!**

Your wallet is now protected with your PIN.
⚠️ **Remember your PIN** - it cannot be recovered!

You will need your PIN for:
• Backing up your seed phrase
• Sending payments
• Changing PIN

Use /help to see available commands."""

pinInvalidLengthMessage = "❌ PIN must be 4-6 digits. Please try again:"
pinInvalidNumericMessage = "❌ PIN must contain only numbers. Please try again:"
pinTooSimpleMessage = "❌ PIN cannot be all the same digit (e.g. 1111). Please try again:"
pinSequentialMessage = "❌ PIN cannot be sequential (e.g. 1234). Please try again:"
pinIncorrectMessage = "❌ Incorrect PIN. Please try again:"
pinLockedMessage = "🔒 Too many failed attempts. PIN locked for 30 minutes."

pinRequiredMessage = """❌ You need to set a PIN first to secure your wallet.

Use /setpin to create your PIN."""

backupRequestPinMessage = "🔐 Enter your PIN to view your seed phrase:"
backupNoPinMessage = """❌ You need to set a PIN first.

Use /setpin to secure your wallet."""
```

Ripeti per le altre lingue (`it.toml`, `es.toml`, ecc.) se le hai.

---

## 6. Aggiornamento Help

Aggiorna il messaggio di help per includere il nuovo comando. Cerca dove definisci il messaggio di `/help` e aggiungi:

```
/setpin - Set or change your wallet PIN
```

---

## Come Applicare Queste Modifiche

### Opzione 1: Manualmente

1. Apri ogni file con un editor
2. Cerca le sezioni indicate
3. Copia e incolla il codice fornito
4. Salva i file

### Opzione 2: Con Patch (se sai usare patch)

Potresti creare file .patch per alcune modifiche se preferisci.

### Opzione 3: Git Merge

Se usi git, potresti creare un branch con le modifiche e fare merge.

---

## Verifica che tutto funzioni

Dopo aver applicato le modifiche:

1. **Compila**:
   ```bash
   go build -o SatsMobiBotNl main.go
   ```

2. **Controlla errori di compilazione**:
   - Se ci sono errori di import, esegui `go mod tidy`
   - Se ci sono errori di sintassi, ricontrolla le modifiche

3. **Testa**:
   ```bash
   ./SatsMobiBotNl
   ```

4. **Verifica log**:
   Dovresti vedere:
   ```
   [Migration] Adding PIN fields to users table
   [Migration] Successfully added PIN fields to users table
   ```

---

## Nota Importante

Queste modifiche sono **esempi generici**. Il tuo codice potrebbe essere strutturato diversamente. 

**Se hai dubbi**:
1. Cerca funzioni simili nel tuo codice
2. Adatta gli esempi alla tua struttura
3. Testa in un ambiente di sviluppo prima di produzione
4. Fai sempre backup prima di modificare file critici

---

## In caso di problemi

Se dopo le modifiche hai errori:

1. **Errori di compilazione**: Controlla che tutti gli import siano corretti
2. **Errori runtime**: Controlla i log per il messaggio d'errore specifico
3. **Migration non parte**: Verifica che la chiamata a `MigratePinFields()` sia nel posto giusto
4. **Comando /setpin non funziona**: Verifica che sia registrato in `OnCommand()`

Se hai bisogno di aiuto, condividi:
- Il messaggio di errore completo
- Il file specifico che stai modificando
- La sezione di codice problematica
