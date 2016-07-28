package db

import (
	"database/sql"
	"path"
	"sync"

	"github.com/OpenBazaar/openbazaar-go/repo"
	"github.com/OpenBazaar/spvwallet"
	"github.com/op/go-logging"
	_ "github.com/mutecomm/go-sqlcipher"
)

var log = logging.MustGetLogger("db")

type SQLiteDatastore struct {
	config          repo.Config
	followers       repo.Followers
	following       repo.Following
	offlineMessages repo.OfflineMessages
	pointers        repo.Pointers
	keys            spvwallet.Keys
	state           spvwallet.State
	stxos           spvwallet.Stxos
	txns            spvwallet.Txns
	utxos           spvwallet.Utxos
	settings        repo.Settings
	db              *sql.DB
	lock            *sync.Mutex
}

func Create(repoPath, password string, testnet bool) (*SQLiteDatastore, error) {
	var dbPath string
	if testnet {
		dbPath = path.Join(repoPath, "datastore", "testnet.db")
	} else {
		dbPath = path.Join(repoPath, "datastore", "mainnet.db")
	}
	conn, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, err
	}
	if password != "" {
		p := "pragma key = '" + password + "';"
		conn.Exec(p)
	}

	l := new(sync.Mutex)
	sqliteDB := &SQLiteDatastore{
		config: &ConfigDB{
			db:   conn,
			lock: l,
			path: dbPath,
		},
		followers: &FollowerDB{
			db:   conn,
			lock: l,
		},
		following: &FollowingDB{
			db:   conn,
			lock: l,
		},
		offlineMessages: &OfflineMessagesDB{
			db:   conn,
			lock: l,
		},
		pointers: &PointersDB{
			db:   conn,
			lock: l,
		},
		keys: &KeysDB{
			db:   conn,
			lock: l,
		},
		state: &StateDB{
			db:   conn,
			lock: l,
		},
		stxos: &StxoDB{
			db:   conn,
			lock: l,
		},
		txns: &TxnsDB{
			db:   conn,
			lock: l,
		},
		utxos: &UtxoDB{
			db:   conn,
			lock: l,
		},
		settings: &SettingsDB{
			db:   conn,
			lock: l,
		},
		db:   conn,
		lock: l,
	}

	return sqliteDB, nil
}

func (d *SQLiteDatastore) Close() {
	d.db.Close()
}

func (d *SQLiteDatastore) Config() repo.Config {
	return d.config
}

func (d *SQLiteDatastore) Followers() repo.Followers {
	return d.followers
}

func (d *SQLiteDatastore) Following() repo.Following {
	return d.following
}

func (d *SQLiteDatastore) OfflineMessages() repo.OfflineMessages {
	return d.offlineMessages
}

func (d *SQLiteDatastore) Pointers() repo.Pointers {
	return d.pointers
}

func (d *SQLiteDatastore) Keys() spvwallet.Keys {
	return d.keys
}

func (d *SQLiteDatastore) State() spvwallet.State {
	return d.state
}

func (d *SQLiteDatastore) Stxos() spvwallet.Stxos {
	return d.stxos
}

func (d *SQLiteDatastore) Txns() spvwallet.Txns {
	return d.txns
}

func (d *SQLiteDatastore) Utxos() spvwallet.Utxos {
	return d.utxos
}

func (d *SQLiteDatastore) Settings() repo.Settings {
	return d.settings
}

func (d *SQLiteDatastore) Copy(dbPath string, password string) error {
	d.lock.Lock()
	defer d.lock.Unlock()
	var cp string
	stmt := "select name from sqlite_master where type='table'"
	rows, err := d.db.Query(stmt)
	if err != nil {
		log.Error(err)
		return err
	}
	var tables []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return err
		}
		tables = append(tables, name)
	}
	if password == "" {
		cp = `attach database '` + dbPath + `' as plaintext key '';`
		for _, name := range tables {
			cp = cp + "insert into plaintext." + name + " select * from main." + name + ";"
		}
	} else {
		cp = `attach database '` + dbPath + `' as encrypted key '` + password + `';`
		for _, name := range tables {
			cp = cp + "insert into encrypted." + name + " select * from main." + name + ";"
		}
	}

	_, err = d.db.Exec(cp)
	if err != nil {
		return err
	}

	return nil
}

func initDatabaseTables(db *sql.DB, password string) error {
	var sqlStmt string
	if password != "" {
		sqlStmt = "PRAGMA key = '" + password + "';"
	}
	sqlStmt = sqlStmt + `
	PRAGMA user_version = 0;
	create table config (key text primary key not null, value blob);
	create table followers (peerID text primary key not null);
	create table following (peerID text primary key not null);
	create table offlinemessages (url text primary key not null, timestamp integer);
	create table pointers (pointerID text primary key not null, key text, address text, purpose integer, timestamp integer);
	create table keys (scriptPubKey text primary key not null, purpose integer, keyIndex integer, used integer);
	create table utxos (outpoint text primary key not null, value integer, height integer, scriptPubKey text);
	create table stxos (outpoint text primary key not null, value integer, height integer, scriptPubKey text, spendHeight integer, spendTxid text);
	create table txns (txid text primary key not null, tx blob);
	create table state (key text primary key not null, value text);
	`
	_, err := db.Exec(sqlStmt)
	if err != nil {
		return err
	}
	return nil
}

type ConfigDB struct {
	db   *sql.DB
	lock *sync.Mutex
	path string
}

func (c *ConfigDB) Init(mnemonic string, identityKey []byte, password string) error {
	c.lock.Lock()
	defer c.lock.Unlock()
	if err := initDatabaseTables(c.db, password); err != nil {
		return err
	}
	tx, err := c.db.Begin()
	if err != nil {
		return err
	}
	stmt, err := tx.Prepare("insert into config(key, value) values(?,?)")
	if err != nil {
		return err
	}
	defer stmt.Close()
	_, err = stmt.Exec("mnemonic", mnemonic)
	if err != nil {
		tx.Rollback()
		return err
	}
	_, err = stmt.Exec("identityKey", identityKey)
	if err != nil {
		tx.Rollback()
		return err
	}
	tx.Commit()
	return nil

}

func (c *ConfigDB) GetMnemonic() (string, error) {
	c.lock.Lock()
	defer c.lock.Unlock()
	stmt, err := c.db.Prepare("select value from config where key=?")
	defer stmt.Close()
	var mnemonic string
	err = stmt.QueryRow("mnemonic").Scan(&mnemonic)
	if err != nil {
		log.Fatal(err)
	}
	return mnemonic, nil
}

func (c *ConfigDB) GetIdentityKey() ([]byte, error) {
	c.lock.Lock()
	defer c.lock.Unlock()
	stmt, err := c.db.Prepare("select value from config where key=?")
	defer stmt.Close()
	var identityKey []byte
	err = stmt.QueryRow("identityKey").Scan(&identityKey)
	if err != nil {
		log.Fatal(err)
	}
	return identityKey, nil
}

func (c *ConfigDB) IsEncrypted() bool {
	pwdCheck := "select count(*) from sqlite_master;"
	_, err := c.db.Exec(pwdCheck) // fails is wrong pw entered
	if err != nil {
		return true
	}
	return false
}
