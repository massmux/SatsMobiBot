# Installazione Sistema PIN per SatsMobi Bot

## 📋 Panoramica

Questo update aggiunge un sistema di protezione tramite PIN per i wallet self-custodial degli utenti. Con questo sistema:
- Ogni utente imposta un PIN personale (4-6 cifre)
- La seedphrase viene cifrata con una chiave derivata dal PIN dell'utente
- Solo l'utente con il proprio PIN può accedere alla propria seedphrase
- Nemmeno l'amministratore del bot può decifrare le seedphrase

## 📦 File da Installare

### 1. File NUOVI da aggiungere:

```
internal/breez/pin.go                    → Gestione crittografia PIN
internal/telegram/pin.go                 → Handler Telegram per PIN  
internal/database/pin_migration.go       → Migration database
```

### 2. File da SOSTITUIRE:

```
internal/lnbits/types.go                 → Aggiunge campi PIN alla struct User
internal/telegram/backup.go              → Aggiunge richiesta PIN per backup
```

## 🔧 Istruzioni di Installazione

### Passo 1: Backup del Database

**IMPORTANTE**: Prima di procedere, fai un backup completo del database!

```bash
# Se usi SQLite
cp data/satsmobi.db data/satsmobi.db.backup

# Se usi PostgreSQL
pg_dump your_database > backup.sql
```

### Passo 2: Copia i Nuovi File

```bash
# Da questa directory, copia i file nella posizione corretta:

# File nuovi:
cp pin.go /path/to/SatsMobiBotNl/internal/breez/pin.go
cp telegram_pin.go /path/to/SatsMobiBotNl/internal/telegram/pin.go
cp pin_migration.go /path/to/SatsMobiBotNl/internal/database/pin_migration.go

# File da sostituire (fai backup prima!):
cp types.go /path/to/SatsMobiBotNl/internal/lnbits/types.go
cp backup.go /path/to/SatsMobiBotNl/internal/telegram/backup.go
```

### Passo 3: Modifica il File Handler

Aggiungi queste righe a `internal/telegram/handler.go`:

#### Nella funzione `OnCommand()`:

```go
func (bot *TipBot) OnCommand() {
	// ... existing commands ...
	
	// PIN management
	bot.Telegram.Handle("/setpin", bot.anyTextHandler(bot.setPinHandler))
	
	// ... rest of commands ...
}
```

#### Nella funzione che gestisce gli stati (es. `handleMessageWithState`):

```go
switch user.StateKey {
	// ... existing cases ...
	
	case lnbits.UserStateEnterNewPin,
		lnbits.UserStateConfirmNewPin,
		lnbits.UserStateEnterPinForOperation,
		lnbits.UserStateEnterPinForBackup,
		lnbits.UserStateEnterPinForPayment:
		return bot.handlePinInput(ctx)
	
	// ... other cases ...
}
```

### Passo 4: Aggiungi la Migration

Nel file `internal/database/migrations.go` (o dove gestisci le migration), aggiungi:

```go
import (
	"github.com/massmux/SatsMobiBot/internal/database"
)

func RunMigrations(db *gorm.DB) error {
	// ... existing migrations ...
	
	// Add PIN fields migration
	if err := database.MigratePinFields(db); err != nil {
		return fmt.Errorf("failed to run PIN migration: %w", err)
	}
	
	// ... rest of migrations ...
	return nil
}
```

### Passo 5: Installa Dipendenze

```bash
cd /path/to/SatsMobiBotNl
go get golang.org/x/crypto/pbkdf2
go get golang.org/x/crypto/bcrypt
go mod tidy
```

### Passo 6: Compila e Testa

```bash
# Compila
go build -o SatsMobiBotNl main.go

# Testa in ambiente di sviluppo/testnet
./SatsMobiBotNl
```

### Passo 7: Verifica le Migration

Al primo avvio, dovresti vedere nei log:

```
[Migration] Adding PIN fields to users table
[Migration] Successfully added PIN fields to users table
[Migration] Database statistics:
  - Total users: XX
  - Users with mnemonic: YY
  - Users will be prompted to set PIN on next wallet access
```

## 🧪 Testing

### Test Manuale

1. **Nuovo Utente**:
   ```
   /start
   /setpin
   [inserisci 1234]
   [conferma 1234]
   /backup
   [inserisci 1234]
   ```

2. **Cambio PIN**:
   ```
   /setpin
   [inserisci vecchio PIN]
   [inserisci nuovo PIN]
   [conferma nuovo PIN]
   ```

3. **PIN Errato**:
   ```
   /backup
   [inserisci PIN sbagliato 5 volte]
   # Dovrebbe bloccarsi per 30 minuti
   ```

### Test Unitari

```bash
cd internal/breez
go test -v ./...
```

## 🔐 Sicurezza

### Parametri di Sicurezza Usati:

- **PBKDF2**: 100,000 iterazioni con SHA-256
- **AES-256-GCM**: Cifratura autenticata
- **Bcrypt**: Cost factor 12 per hash PIN
- **Salt**: 32 bytes casuali per ogni utente
- **Rate Limiting**: 5 tentativi prima del lockout di 30 minuti

### Best Practices:

1. ✅ I messaggi con PIN vengono cancellati immediatamente
2. ✅ La seedphrase viene mostrata solo per 60 secondi
3. ✅ Nessun PIN o seedphrase nei log
4. ✅ Lockout temporaneo dopo troppi tentativi falliti

## 🔄 Migrazione Utenti Esistenti

Gli utenti esistenti con seedphrase cifrate con la master key continueranno a funzionare:

1. Al primo `/backup`, verrà chiesto di impostare un PIN
2. Una volta impostato il PIN, la seedphrase viene automaticamente ricifrata
3. Da quel momento in poi, solo il PIN dell'utente può decifrare la seedphrase

**Opzione di migrazione forzata**: Se vuoi richiedere a tutti gli utenti di impostare un PIN, modifica `internal/telegram/start.go` per bloccare l'accesso finché non viene impostato un PIN.

## 📱 Comandi Utente

Nuovi comandi disponibili:

- `/setpin` - Imposta o cambia il PIN
- `/backup` - Visualizza la seedphrase (richiede PIN)

## ⚠️ Avvertenze Importanti

### Per l'Amministratore:

1. **Backup**: Fai sempre backup del database prima di applicare modifiche
2. **Testing**: Testa in testnet prima di produzione
3. **Recovery**: Se un utente dimentica il PIN, NON può essere recuperato
4. **Comunicazione**: Informa chiaramente gli utenti che il PIN non è recuperabile

### Per gli Utenti:

1. **PIN non recuperabile**: Se dimenticano il PIN, perdono accesso al wallet
2. **Backup obbligatorio**: Devono salvare la seedphrase offline
3. **PIN sicuro**: Deve essere memorabile ma non ovvio (no 1234, 0000, ecc.)

## 🐛 Troubleshooting

### Errore: "ENCRYPTION_KEY environment variable is required"

La master key è ancora necessaria per backward compatibility. Assicurati di:
```bash
export ENCRYPTION_KEY=$(cat /path/to/encryption.key)
```

### Errore: "failed to migrate PIN fields"

Verifica che il database sia accessibile e che l'utente abbia permessi di ALTER TABLE.

### Utente non riesce a impostare PIN

1. Verifica che i campi PIN siano stati aggiunti al database
2. Controlla i log per errori specifici
3. Verifica che l'utente abbia un wallet inizializzato

### Migration non parte

Assicurati di aver aggiunto la chiamata a `MigratePinFields()` nel punto corretto del codice di migration.

## 📊 Monitoring

### Log da Monitorare:

```
[PIN] PIN saved successfully for user X
[PIN] Mnemonic encrypted with PIN-derived key
[PIN] Mnemonic decrypted with PIN-derived key
[PIN] Too many failed attempts - user locked
```

### Metriche Consigliate:

- Numero di utenti con PIN impostato
- Numero di lockout per tentativi falliti
- Tempo medio per impostare un PIN

## 🔄 Rollback

Se necessario fare rollback:

1. Ripristina il backup del database
2. Ripristina i file vecchi:
   ```bash
   git checkout internal/lnbits/types.go
   git checkout internal/telegram/backup.go
   ```
3. Rimuovi i file nuovi:
   ```bash
   rm internal/breez/pin.go
   rm internal/telegram/pin.go
   rm internal/database/pin_migration.go
   ```
4. Ricompila

## 📞 Supporto

In caso di problemi:

1. Controlla i log del bot
2. Verifica che tutte le dipendenze siano installate
3. Assicurati che la migration sia stata eseguita con successo
4. Controlla che la master key sia ancora configurata correttamente

## ✅ Checklist Post-Installazione

```
[ ] Backup database completato
[ ] File nuovi copiati
[ ] File esistenti sostituiti
[ ] Handler modificato
[ ] Migration aggiunta
[ ] Dipendenze installate
[ ] Compilazione riuscita
[ ] Migration database eseguita con successo
[ ] Test con nuovo utente completato
[ ] Test cambio PIN completato
[ ] Test lockout completato
[ ] Documentazione utenti aggiornata
[ ] Monitoraggio configurato
```

## 📝 Note Finali

- Il sistema è progettato per essere **backward compatible**
- Gli utenti esistenti possono continuare a usare il bot normalmente
- La migrazione al PIN avviene gradualmente e automaticamente
- Non è necessario interrompere il servizio per l'installazione (con le dovute precauzioni)

**Data versione**: 2025-01-31
**Versione PIN System**: 1.0.0
