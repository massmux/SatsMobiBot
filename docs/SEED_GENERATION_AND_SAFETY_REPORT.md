# Seed generation & safety report (Breez “Safe” wallet)

This document describes **how the Breez wallet seed phrase (mnemonic) is generated, stored, and revealed**, and gives a quick assessment of whether the current approach is “safe enough” and what the main risks are.

## What is generated

- **Mnemonic format**: BIP39 mnemonic phrase (currently **12 words**).
- **Entropy**: **128 bits** (16 bytes) for 12 words.
- **Passphrase**: empty string (no BIP39 passphrase is used).

Where this is implemented:
- `internal/breez/wallet.go` (`GenerateMnemonic()`)

## How the mnemonic is generated

The bot generates the mnemonic as follows:

1. Allocate 16 bytes (128 bits) of entropy.
2. Fill entropy using **`crypto/rand`** (cryptographically secure randomness from the OS).
3. Convert entropy into a **BIP39 mnemonic** using `github.com/tyler-smith/go-bip39`.

This is the right class of approach for mnemonic generation: **CSPRNG → BIP39 mnemonic**.

Where this happens:
- `internal/breez/wallet.go` (`GenerateMnemonic()` uses `rand.Read()` + `bip39.NewMnemonic()`).

## When a mnemonic is created for a user

Mnemonic creation happens during Telegram `/start` flow when:

- Breez integration is enabled (`breez.enabled: true`), and
- the user does not already have a stored Breez mnemonic.

Flow:
- `/start` → `initWallet()` → `initUserBreezWallet()`
- If `user.BreezMnemonic == ""`, the bot generates a new mnemonic, encrypts it, and stores it in the DB.

Where this happens:
- `internal/telegram/start.go` (`initWallet`, `initUserBreezWallet`)

## How the mnemonic is stored (at rest)

The mnemonic is stored in the user record as `BreezMnemonic` (string field on `lnbits.User`), but **stored encrypted**.

Encryption details:
- **Algorithm**: AES-256-GCM (AEAD).
- **Key**: `breez.encryption_key` from `config.yaml` (must be **64 hex chars** = 32 bytes).
- **Nonce**: randomly generated per-encryption using `crypto/rand` (`io.ReadFull(rand.Reader, nonce)`).
- **Stored format**: hex-encoded string of `nonce || ciphertext+tag` (GCM output).

Where this happens:
- `internal/breez/encryption.go` (`EncryptMnemonic`, `DecryptMnemonic`)
- `internal/config.go` enforces the key presence/length when Breez is enabled.

## How the mnemonic is used (in memory)

On wallet init:
- the bot decrypts the mnemonic (if present),
- then initializes the Breez SDK client with the mnemonic.

Where this happens:
- `internal/telegram/start.go` (`initUserBreezWallet` → `initializeUserBreezClient(...)`)
- `internal/telegram/bot.go` (reinitialization path: decrypt + init if client missing)

## When the mnemonic is revealed to the user

The user can retrieve the seed phrase via `/backup`:

- Only allowed in **private chat**.
- Bot decrypts mnemonic and sends it in a message.
- Bot **deletes that message after 60 seconds**.
- Bot logs that the user viewed the seed phrase (does not log the mnemonic itself).

Where this happens:
- `internal/telegram/backup.go` (`backupHandler`)

Important note: Telegram messages are **not end-to-end encrypted** in standard cloud chats. Even if the bot deletes the message after 60 seconds, the seed phrase can be copied/forwarded/screenshotted.

## Migration behavior (plaintext → encrypted)

There is migration logic for older databases that might have stored the mnemonic **in plaintext**:

- On decrypt failure, the code checks if the stored string “looks like” a mnemonic.
- If yes, it encrypts it and overwrites `user.BreezMnemonic` with the encrypted value.

Where this happens:
- `internal/telegram/start.go` (`initUserBreezWallet`)
- `internal/telegram/backup.go` (`backupHandler`)
- `internal/telegram/bot.go` (client reinit path)

### ⚠️ Weakness in mnemonic validation during migration

`breez.ValidateMnemonic(...)` currently checks **only the word count** (12/15/18/21/24). It does **not** verify the BIP39 checksum (e.g., using `bip39.IsMnemonicValid`).

Impact:
- A random 12-word string could be treated as “valid plaintext mnemonic” during migration.
- The system could then encrypt and persist garbage, and Breez initialization might fail later.

This is more of a **robustness/safety** issue than a direct cryptographic weakness, but it’s worth fixing.

Where this happens:
- `internal/breez/wallet.go` (`ValidateMnemonic`)

## Checksum validation (requested hardening)

The code now validates the mnemonic using **BIP39 checksum** (not just word count):

- `breez.ValidateMnemonic(...)` normalizes whitespace, checks valid word counts, then calls `bip39.IsMnemonicValid(...)`.
- This validation is enforced:
  - immediately after generating a mnemonic, and
  - before initializing any Breez SDK client with a mnemonic, and
  - before revealing the mnemonic via `/backup`, and
  - before re-initializing a user Breez client from stored mnemonic.

This ensures we never run a user wallet (or show a seed) if the mnemonic fails checksum validation.

## Safety assessment (high-level)

### What looks good

- **Mnemonic generation** uses OS CSPRNG (`crypto/rand`) + BIP39. This is standard and safe.
- **At-rest encryption** uses AES-256-GCM with random nonces, which is a strong modern choice.
- Breez encryption key is validated to be **32 bytes** (64 hex chars) when Breez is enabled.
- The bot does not print the mnemonic in logs (based on current code paths examined).

### Main risks / assumptions

- **Single server-wide encryption key** (`breez.encryption_key`):
  - If the server is compromised or `config.yaml` leaks, an attacker can decrypt **all users’ Breez mnemonics** from the DB.
  - There is no per-user key separation and no key rotation mechanism shown in code.

- **Seed phrase delivery over Telegram** (`/backup`):
  - Not E2EE in normal Telegram bot chats.
  - Deleting after 60 seconds helps reduce accidental exposure but does not prevent copying/screenshotting or Telegram-side retention.

- **12-word mnemonic**:
  - 12 words (128-bit entropy) is commonly acceptable for Bitcoin wallets, but 24 words provides a larger security margin.
  - The codebase already contains `GenerateMnemonic24()` but it is not used for user creation.

- **Migration validation is too weak**:
  - Word-count-only validation is insufficient to confirm a mnemonic is actually valid BIP39.

## Bottom line

- **Generation is correct and uses a safe randomness source**.
- **Storage is encrypted and uses a good AEAD scheme**, but overall security strongly depends on keeping `breez.encryption_key` safe.
- **The biggest practical exposure is `/backup`**, because it transmits the seed phrase via Telegram messages.

## Adding a PIN / “second factor” for seed phrase access (research & difficulty)

### First: what problem are we trying to solve?

There are two common threat models:

- **DB leak / backups leak** (attacker gets DB contents, but not runtime secrets or user interactions).
- **Full server compromise** (attacker gets DB + `breez.encryption_key` and/or can modify the bot and intercept user inputs).

A PIN/2FA mechanism can help a lot against **DB leak**, but **cannot fully protect against full server compromise** (because the server can always capture whatever the user types).

### Option A (most practical): PIN-gated envelope encryption (recommended)

Goal: even if the DB leaks, the attacker still can’t decrypt mnemonics without the user PIN.

High-level design:

- **Per user**, generate a random **data-encryption key (DEK)** to encrypt the mnemonic (AES-GCM).
- Encrypt (“wrap”) the DEK using a key derived from:
  - the server secret (`breez.encryption_key`, or better: a separate `seed_kek`), and
  - a **user PIN** processed through a strong KDF (Argon2id / scrypt) with a per-user salt.
- Store in DB:
  - encrypted mnemonic (with DEK),
  - wrapped DEK,
  - KDF params + salt,
  - optionally a PIN verifier hash for fast PIN checks.

Runtime behavior:

- Normal operations that require Breez (payments, swaps) need the mnemonic. You have to choose one UX model:
  - **Unlocked session model (best UX)**: user enters PIN once; bot keeps the decrypted DEK/mnemonic **in memory** for some TTL (e.g., 30 minutes) and re-locks after inactivity/restart.
  - **Always prompt model (most secure, worst UX)**: prompt PIN every time Breez is needed (likely unacceptable).

Security notes:
- Helps strongly against **DB-only compromise**.
- Does **not** help if the attacker has control of the server process (they can capture PIN).

Difficulty estimate in this codebase:
- **Medium**. Requires new DB fields on `lnbits.User` (salt/params/wrapped key), a `/setpin` and `/pin` flow, and in-memory “unlock” cache.
- Rough effort: ~1–3 days depending on UX and migration needs.

### Option B: protect the server key using a real KMS/HSM (helps a different threat)

If the main worry is that `breez.encryption_key` sitting in config is risky, you can:

- move encryption keys out of `config.yaml`,
- load them from environment variables, and ideally
- use a managed KMS (AWS KMS / GCP KMS) or OS keychain/TPM-backed storage.

This does **not** add a user second factor, but it materially improves:
- accidental leaks (config file backups),
- basic disk exfiltration,
- operational key rotation.

Difficulty: Low–Medium (mostly plumbing + deployment changes).

### Option C: “2FA” via Telegram/OTP

Classic TOTP (Google Authenticator) can be implemented, but note:

- If the bot is compromised, it can still ask for/steal OTP codes.
- This is still useful mainly against **DB leak** and some “offline attacker” scenarios.

In practice, a **PIN (Option A)** is usually simpler than full TOTP UX in a Telegram bot.

### Option D: don’t ever show the seed phrase again

You could decide:
- show seed only once at creation (and never again),
- require the user to store it securely.

But users will lose it, and support burden increases. Also, this does not address DB/server compromise; it’s just a UX/security trade.



