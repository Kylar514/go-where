package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
)

func serverCommand(args []string) error {
	fs := flag.NewFlagSet("go-where-server", flag.ContinueOnError)
	dir := fs.String("data", "./server-data", "server metadata and inventories directory")
	address := fs.String("listen", "127.0.0.1:8080", "listen address")
	cert := fs.String("tls-cert", "", "TLS certificate PEM")
	key := fs.String("tls-key", "", "TLS private key PEM path")
	if err := fs.Parse(args); err != nil {
		return err
	}
	s, err := openInventoryServer(*dir)
	if err != nil {
		return err
	}
	defer s.db.Close()
	a := fs.Args()
	if len(a) == 0 {
		return fmt.Errorf("commands: serve; user create/disable/grant; token create/list/revoke; inventory create/add/list. Global flags precede the command")
	}
	if len(a) > 1 && a[1] != "list" && a[0] != "serve" {
		lock, err := os.OpenFile(filepath.Join(*dir, "admin.lock"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			return fmt.Errorf("another administrator operation is active")
		}
		lock.Close()
		defer os.Remove(filepath.Join(*dir, "admin.lock"))
	}
	if a[0] == "serve" {
		lock, e := os.OpenFile(filepath.Join(*dir, "serve.lock"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if e != nil {
			return fmt.Errorf("server data directory is already locked; remove serve.lock only after verifying no server is running")
		}
		lock.Close()
		defer os.Remove(filepath.Join(*dir, "serve.lock"))
		return s.serve(*address, *cert, *key)
	}
	if len(a) < 2 {
		return fmt.Errorf("subcommand required")
	}
	userID := func(name string) (string, error) {
		var id string
		e := s.db.QueryRow("SELECT id FROM users WHERE name=?", name).Scan(&id)
		return id, e
	}
	switch a[0] + " " + a[1] {
	case "user create":
		if len(a) != 3 || !inventoryName(a[2]) {
			return fmt.Errorf("user create NAME")
		}
		_, err = s.db.Exec("INSERT INTO users(id,name) VALUES(?,?)", inventoryID(), a[2])
		return err
	case "user disable":
		if len(a) != 3 {
			return fmt.Errorf("user disable NAME")
		}
		id, e := userID(a[2])
		if e != nil {
			return e
		}
		_, err = s.db.Exec("UPDATE users SET disabled=1 WHERE id=?", id)
		return err
	case "user grant":
		if len(a) != 5 {
			return fmt.Errorf("user grant USER INVENTORY ACCESS (read-only/read-write/none)")
		}
		id, e := userID(a[2])
		if e != nil {
			return e
		}
		var inventory string
		if e = s.db.QueryRow("SELECT id FROM inventories WHERE name=? OR id=?", a[3], a[3]).Scan(&inventory); e != nil {
			return e
		}
		if a[4] == "none" {
			_, err = s.db.Exec("DELETE FROM grants WHERE user_id=? AND inventory_id=?", id, inventory)
			return err
		}
		if a[4] != "read-only" && a[4] != "read-write" {
			return fmt.Errorf("invalid access")
		}
		_, err = s.db.Exec("INSERT INTO grants(user_id,inventory_id,access) VALUES(?,?,?) ON CONFLICT(user_id,inventory_id) DO UPDATE SET access=excluded.access", id, inventory, a[4])
		return err
	case "token create":
		if len(a) < 4 || len(a) > 5 {
			return fmt.Errorf("token create USER DEVICE [DAYS]")
		}
		id, e := userID(a[2])
		if e != nil {
			return e
		}
		days := 90
		if len(a) == 5 {
			if _, e = fmt.Sscan(a[4], &days); e != nil {
				return e
			}
		}
		v, e := s.issueToken(id, a[3], days)
		if e != nil {
			return e
		}
		fmt.Fprintln(os.Stdout, v.Secret)
		return nil
	case "token revoke":
		if len(a) != 3 {
			return fmt.Errorf("token revoke TOKEN_ID")
		}
		_, err = s.db.Exec("UPDATE tokens SET revoked=1 WHERE id=?", a[2])
		return err
	case "token list":
		if len(a) != 3 {
			return fmt.Errorf("token list USER")
		}
		id, e := userID(a[2])
		if e != nil {
			return e
		}
		rows, e := s.db.Query("SELECT id,name,expires,revoked FROM tokens WHERE user_id=?", id)
		if e != nil {
			return e
		}
		defer rows.Close()
		for rows.Next() {
			var id, name string
			var expires int64
			var revoked int
			if e = rows.Scan(&id, &name, &expires, &revoked); e != nil {
				return e
			}
			fmt.Printf("%s %s expiry=%d revoked=%d\n", id, name, expires, revoked)
		}
		return rows.Err()
	case "inventory create", "inventory add":
		if len(a) < 3 || !inventoryName(a[2]) {
			return fmt.Errorf("inventory create NAME; inventory add NAME PATH")
		}
		var source InventorySource
		if a[1] == "create" && len(a) == 3 {
			source, err = createInventory(a[2], filepath.Join(*dir, a[2]+".db"))
		} else if a[1] == "add" && len(a) == 4 {
			source, err = attachInventory(a[2], a[3])
		} else {
			return fmt.Errorf("invalid inventory arguments")
		}
		if err != nil {
			return err
		}
		p, e := sourcePath(source)
		if e != nil {
			return e
		}
		_, err = s.db.Exec("INSERT INTO inventories(id,name,path) VALUES(?,?,?)", source.ID, source.Name, p)
		if err == nil {
			fmt.Println(source.ID)
		}
		return err
	case "inventory list":
		rows, e := s.db.Query("SELECT id,name FROM inventories ORDER BY name")
		if e != nil {
			return e
		}
		defer rows.Close()
		for rows.Next() {
			var id, name string
			if e = rows.Scan(&id, &name); e != nil {
				return e
			}
			fmt.Println(id, name)
		}
		return rows.Err()
	}
	return fmt.Errorf("unknown server command")
}
