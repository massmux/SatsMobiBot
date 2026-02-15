#!/bin/bash
# Salva come check_db.sh

DB_PATH="./data/bot.db"

echo "=== SATSMOBI DATABASE VERIFICATION ==="
echo ""

sqlite3 $DB_PATH << 'EOF'
.headers on
.mode column

SELECT '=== TOTAL USERS ===' as info;
SELECT COUNT(*) as total FROM users;

SELECT '' as spacer;
SELECT '=== USERS WITH PIN ===' as info;
SELECT COUNT(*) as with_pin FROM users WHERE pin_hash != '';

SELECT '' as spacer;
SELECT '=== USERS WITH BREEZ ===' as info;
SELECT COUNT(*) as with_breez FROM users WHERE breez_initialized = 1;

SELECT '' as spacer;
SELECT '=== USERS RECAP ===' as info;
SELECT 
    name as telegram_id,
    CASE WHEN initialized THEN 'YES' ELSE 'NO' END as init,
    CASE WHEN breez_initialized THEN 'YES' ELSE 'NO' END as breez,
    CASE WHEN pin_hash != '' THEN 'YES' ELSE 'NO' END as has_pin,
    CASE WHEN breez_mnemonic != '' THEN 'YES' ELSE 'NO' END as has_mnemonic
FROM users
LIMIT 10;
EOF
