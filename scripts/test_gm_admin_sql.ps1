param([string]$ContainerName = 'l1jgo-whale-postgres-1')

# 僅建立交易內的獨立 schema 和固定 fixture，最後 ROLLBACK；不讀取正式資料。
$ErrorActionPreference = 'Stop'
$taskRoot = Split-Path -Parent $PSScriptRoot
$taskSql = [System.Collections.Generic.List[string]]::new()
$taskSql.Add('BEGIN; CREATE SCHEMA codex_gm_admin_validation_20261013; SET LOCAL search_path TO codex_gm_admin_validation_20261013;')
foreach ($taskName in @('001_phase1.sql', '005_warehouse.sql', '029_warehouse_item_state.sql', '032_gm_admin.sql', '034_gm_advanced_polymorph.sql')) {
    $taskMigration = Get-Content -Raw -LiteralPath (Join-Path $taskRoot ('internal/persist/migrations/' + $taskName))
    $taskSql.Add(($taskMigration -split '(?m)^-- \+goose Down')[0])
}
$taskRepo = Get-Content -Raw -LiteralPath (Join-Path $taskRoot 'internal/persist/gm_admin.go')
$taskQueries = @([regex]::Matches($taskRepo, '`([^`]+)`', [System.Text.RegularExpressions.RegexOptions]::Singleline) | ForEach-Object { $_.Groups[1].Value })
if ($taskQueries.Count -ne 14) { throw "Unexpected repository SQL count: $($taskQueries.Count)" }
$taskSql.Add(@'
INSERT INTO accounts(name,password_hash) VALUES ('fixture_a','unused'),('fixture_b','unused'),('fixture_c','unused');
INSERT INTO characters(id,account_name,name,class_type,sex,class_id,str,dex,con,wis,cha,intel,hp,mp,max_hp,max_mp,level,home_town_id,contribution) VALUES
(1,'fixture_a','fixture_a_low',1,0,61,10,10,10,10,10,10,100,20,100,20,10,1,10),
(2,'fixture_a','fixture_a_high',1,0,61,10,10,10,10,10,10,100,20,100,20,60,1,30),
(3,'fixture_b','fixture_b_high',1,0,61,10,10,10,10,10,10,100,20,100,20,70,1,60),
(4,'fixture_c','fixture_c_high',1,0,61,10,10,10,10,10,10,100,20,100,20,90,2,0);
UPDATE towns SET tax_rate=2,tax_rate_reserved=5,sales_money=1000,town_fix_tax=101 WHERE town_id=1;
'@)
$taskSql.Add("PREPARE gm_account_ban(text,boolean) AS $($taskQueries[0]);")
$taskSql.Add("PREPARE gm_ip_lookup(text) AS $($taskQueries[1]);")
$taskSql.Add("PREPARE gm_ip_ban(text,text) AS $($taskQueries[2]);")
$taskSql.Add("PREPARE gm_ip_unban(text) AS $($taskQueries[3]);")
$taskSql.Add("PREPARE gm_account_gift(text) AS $($taskQueries[4]);")
$taskSql.Add("PREPARE gm_level_gift(smallint,smallint) AS $($taskQueries[5]);")
$taskSql.Add("PREPARE gm_gift_insert(text,integer,integer,smallint,smallint,integer,smallint) AS $($taskQueries[6]);")
$taskSql.Add(@'
EXECUTE gm_account_ban('fixture_a',true);
EXECUTE gm_ip_ban('192.0.2.42','fixture_a');
EXECUTE gm_ip_ban('192.0.2.42','fixture_a');
EXECUTE gm_ip_lookup('192.0.2.42');
EXECUTE gm_ip_ban('2001:db8::1','fixture_a');
DO $check$ BEGIN
IF NOT (SELECT banned FROM accounts WHERE name='fixture_a') THEN RAISE EXCEPTION 'account ban failed'; END IF;
IF (SELECT COUNT(*) FROM banned_ips) <> 2 THEN RAISE EXCEPTION 'IP uniqueness or IPv6 failed'; END IF;
END $check$;
EXECUTE gm_ip_unban('192.0.2.42');
DO $check$ BEGIN
IF EXISTS(SELECT 1 FROM banned_ips WHERE ip='192.0.2.42'::inet) THEN RAISE EXCEPTION 'IP unban failed'; END IF;
END $check$;
EXECUTE gm_account_gift('*');
EXECUTE gm_level_gift(45,80);
EXECUTE gm_gift_insert('fixture_a',40010,50,0,0,500000001,0);
EXECUTE gm_gift_insert('fixture_b',40010,50,0,0,500000002,0);
EXECUTE gm_gift_insert('fixture_a',31,1,7,1,500000003,12);
EXECUTE gm_gift_insert('fixture_a',31,1,7,1,500000004,12);
DO $check$ BEGIN
IF (SELECT COUNT(*) FROM warehouse_items WHERE account_name='fixture_a' AND item_id=31 AND count=1 AND enchant_lvl=7 AND bless=1 AND charge_count=12 AND NOT identified)<>2 THEN RAISE EXCEPTION 'non-stack gift attributes failed'; END IF;
IF (SELECT SUM(count) FROM warehouse_items WHERE item_id=40010)<>100 THEN RAISE EXCEPTION 'stack gift amount failed'; END IF;
IF (SELECT COUNT(DISTINCT account_name) FROM characters WHERE level BETWEEN 45 AND 80 AND deleted_at IS NULL)<>2 THEN RAISE EXCEPTION 'level gift account deduplication failed'; END IF;
END $check$;
SAVEPOINT gm_gift_rollback;
EXECUTE gm_gift_insert('fixture_c',40010,50,0,0,500000005,0);
ROLLBACK TO SAVEPOINT gm_gift_rollback;
DO $check$ BEGIN
IF EXISTS(SELECT 1 FROM warehouse_items WHERE account_name='fixture_c') THEN RAISE EXCEPTION 'gift rollback failed'; END IF;
END $check$;
'@)
$taskItemRepo = Get-Content -Raw -LiteralPath (Join-Path $taskRoot 'internal/persist/item_repo.go')
$taskMaxObjectSQL = [regex]::Match($taskItemRepo, '`(SELECT GREATEST[^`]+)`').Groups[1].Value
if (-not $taskMaxObjectSQL) { throw 'Cannot find persisted maximum object ID query' }
$taskSql.Add('CREATE TABLE character_items(obj_id INT); INSERT INTO character_items VALUES(499999999);')
$taskSql.Add('DO $check$ BEGIN IF (' + $taskMaxObjectSQL + ') <> 500000004 THEN RAISE EXCEPTION ''warehouse gift object ID collision after restart''; END IF; END $check$;')
$taskBuffRepo = Get-Content -Raw -LiteralPath (Join-Path $taskRoot 'internal/persist/buff_repo.go')
$taskDeleteBuffSQL = [regex]::Match($taskBuffRepo, '`(DELETE FROM character_buffs WHERE char_id = \$1)`').Groups[1].Value
if (-not $taskDeleteBuffSQL) { throw 'Cannot find saved buff deletion query' }
$taskSql.Add('CREATE TABLE character_buffs(char_id INT,skill_id INT); INSERT INTO character_buffs VALUES(1,1005);')
$taskSql.Add('PREPARE gm_empty_buffs(integer) AS ' + $taskDeleteBuffSQL + '; EXECUTE gm_empty_buffs(1);')
$taskSql.Add('DO $check$ BEGIN IF EXISTS(SELECT 1 FROM character_buffs) THEN RAISE EXCEPTION ''cleared mute remained in saved buffs''; END IF; END $check$;')
$taskAdvPolyRepo = Get-Content -Raw -LiteralPath (Join-Path $taskRoot 'internal/persist/advanced_polymorph_repo.go')
$taskAdvPolyQueries = @([regex]::Matches($taskAdvPolyRepo, '`([^`]+)`', [System.Text.RegularExpressions.RegexOptions]::Singleline) | ForEach-Object { $_.Groups[1].Value })
if ($taskAdvPolyQueries.Count -ne 2) { throw 'Unexpected advanced polymorph query count' }
$taskSql.Add('PREPARE gm_advpoly_unlock(integer,integer) AS ' + $taskAdvPolyQueries[0] + ';')
$taskSql.Add('PREPARE gm_advpoly_reset(integer) AS ' + $taskAdvPolyQueries[1] + ';')
$taskSql.Add(@'
EXECUTE gm_advpoly_unlock(1,13715);
UPDATE character_advanced_polymorphs SET is_default=TRUE WHERE char_id=1 AND polyid=13715;
EXECUTE gm_advpoly_unlock(1,13715);
EXECUTE gm_advpoly_unlock(1,13719);
EXECUTE gm_advpoly_unlock(3,13715);
DO $check$ BEGIN
IF NOT EXISTS(SELECT 1 FROM character_advanced_polymorphs WHERE char_id=1 AND polyid=13715 AND unlock_count=2 AND is_default) THEN RAISE EXCEPTION 'advanced polymorph increment/default retention failed'; END IF;
END $check$;
EXECUTE gm_advpoly_reset(1);
DO $check$ BEGIN
IF EXISTS(SELECT 1 FROM character_advanced_polymorphs WHERE char_id=1) OR NOT EXISTS(SELECT 1 FROM character_advanced_polymorphs WHERE char_id=3) THEN RAISE EXCEPTION 'advanced polymorph reset isolation failed'; END IF;
END $check$;
'@)
$taskSql.Add($taskQueries[7] + ';')
$taskSql.Add(@'
DO $check$ BEGIN
IF NOT EXISTS(SELECT 1 FROM towns WHERE town_id=1 AND tax_rate=5 AND sales_money=0 AND sales_money_yesterday=1000) THEN RAISE EXCEPTION 'daily settlement failed'; END IF;
END $check$;
UPDATE towns SET sales_money=2000 WHERE town_id=1;
'@)
foreach ($taskIndex in 8..12) { $taskSql.Add($taskQueries[$taskIndex] + ';') }
$taskSql.Add($taskQueries[13] + ';')
$taskSql.Add(@'
DO $check$ BEGIN
IF NOT EXISTS(SELECT 1 FROM towns WHERE town_id=1 AND leader_id=3 AND leader_name='fixture_b_high' AND tax_rate=0 AND town_fix_tax=0 AND sales_money_yesterday=2000) THEN RAISE EXCEPTION 'monthly leader/reset failed'; END IF;
IF (SELECT town_pay FROM characters WHERE id=1)<>10 OR (SELECT town_pay FROM characters WHERE id=2)<>30 OR (SELECT town_pay FROM characters WHERE id=3)<>60 THEN RAISE EXCEPTION 'monthly proportional payout failed'; END IF;
IF EXISTS(SELECT 1 FROM characters WHERE contribution<>0) THEN RAISE EXCEPTION 'monthly contribution reset failed'; END IF;
END $check$;
SELECT 'GM SQL fixtures: account/IP bans, gift attributes/rollback, hometown daily/monthly passed' AS result;
ROLLBACK;
DO $check$ BEGIN IF EXISTS(SELECT 1 FROM pg_namespace WHERE nspname='codex_gm_admin_validation_20261013') THEN RAISE EXCEPTION 'test schema was not rolled back'; END IF; END $check$;
'@)
$taskSqlText = [string]::Join("`n", $taskSql)
$taskSqlText | docker exec -i $ContainerName sh -c 'exec psql -X -v ON_ERROR_STOP=1 -U "$POSTGRES_USER" -d "$POSTGRES_DB"'
if ($LASTEXITCODE -ne 0) { throw "PostgreSQL fixture validation failed with exit code $LASTEXITCODE" }
