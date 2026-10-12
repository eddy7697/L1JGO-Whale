// gm 是本機角色權限管理工具；沿用伺服器 config/server.toml 與 PG* 環境變數。
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/l1jgo/server/internal/config"
	"github.com/l1jgo/server/internal/persist"
	"go.uber.org/zap"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	defaultConfig := os.Getenv("L1JGO_CONFIG")
	if defaultConfig == "" {
		defaultConfig = "config/server.toml"
	}
	configPath := flag.String("config", defaultConfig, "伺服器設定檔")
	name := flag.String("name", "", "角色完整名稱（必要）")
	level := flag.Int("level", -1, "0=玩家、100=監察員、200=GM（必要）")
	flag.Parse()
	if *name == "" || (*level != 0 && *level != 100 && *level != 200) || flag.NArg() != 0 {
		return fmt.Errorf("用法：go run ./cmd/gm -name 角色名稱 -level 200 [-config config/server.toml]")
	}
	cfg, err := config.Load(*configPath)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	db, err := persist.NewDB(ctx, cfg.Database, zap.NewNop())
	if err != nil {
		return err
	}
	defer db.Close()
	repo := persist.NewCharacterRepo(db)
	ch, err := repo.LoadByName(ctx, *name)
	if err != nil {
		return err
	}
	if ch == nil {
		return fmt.Errorf("角色不存在：%s", *name)
	}
	if err := repo.SetAccessLevel(ctx, ch.Name, int16(*level)); err != nil {
		return err
	}
	fmt.Printf("角色 %s 權限：%d → %d。角色重新登入後生效。\n", ch.Name, ch.AccessLevel, *level)
	return nil
}
