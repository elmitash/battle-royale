package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"battle-royale/internal/config"
	"battle-royale/internal/db"
	"battle-royale/internal/game"
	"battle-royale/internal/handler"
	"battle-royale/internal/view"
)

func main() {
	cfg := config.Load()

	store, err := db.Open(cfg.DBPath)
	if err != nil {
		log.Fatalf("failed to open db: %v", err)
	}
	defer func() {
		if err := store.Close(); err != nil {
			log.Printf("db close error: %v", err)
		}
	}()

	master, err := db.LoadMasterData(cfg.DataDir)
	if err != nil {
		log.Fatalf("failed to load master data: %v", err)
	}
	log.Printf("loaded master data: %d weapons, %d starter items, %d area items, %d recipes, %d NPCs",
		len(master.Weapons), len(master.StartItems), len(master.AreaItems), len(master.Recipes), len(master.NPCs))

	if err := db.EnsureRoundInitialized(context.Background(), store.DB, master, cfg); err != nil {
		log.Fatalf("failed to initialize round: %v", err)
	}

	renderer, err := view.NewRenderer("./templates")
	if err != nil {
		log.Fatalf("failed to parse templates: %v", err)
	}

	engine := game.NewEngine(store.DB, master, cfg)
	schedCtx, schedCancel := context.WithCancel(context.Background())
	defer schedCancel()
	engine.StartScheduler(schedCtx, 30*time.Second)

	authH := &handler.AuthHandler{DB: store.DB, Master: master, View: renderer}
	playH := &handler.PlayHandler{DB: store.DB, Engine: engine, View: renderer}
	adminH := handler.NewAdminHandler(store.DB, master, cfg, engine, renderer)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /", authH.LoginPage)
	mux.HandleFunc("POST /login", authH.LoginSubmit)
	mux.HandleFunc("POST /logout", authH.Logout)
	mux.HandleFunc("GET /regist", authH.RegistPage)
	mux.HandleFunc("POST /regist", authH.RegistSubmit)

	mux.HandleFunc("GET /battle", playH.BattlePage)
	mux.HandleFunc("POST /battle/move", playH.MoveAction)
	mux.HandleFunc("POST /battle/search", playH.SearchAction)
	mux.HandleFunc("POST /battle/attack", playH.AttackAction)
	mux.HandleFunc("POST /battle/runaway", playH.RunawayAction)
	mux.HandleFunc("POST /battle/loot", playH.LootAction)
	mux.HandleFunc("POST /battle/speak", playH.SpeakAction)
	mux.HandleFunc("POST /battle/poisoncheck", playH.PoisonCheckAction)
	mux.HandleFunc("POST /battle/rest", playH.RestAction)
	mux.HandleFunc("POST /battle/firstaid", playH.FirstAidAction)
	mux.HandleFunc("POST /battle/tactics", playH.TacticsAction)
	mux.HandleFunc("POST /battle/item/use", playH.UseItemAction)
	mux.HandleFunc("POST /battle/item/drop", playH.DropItemAction)
	mux.HandleFunc("POST /battle/item/combine", playH.CombineItemAction)

	mux.HandleFunc("GET /map", playH.MapPage)
	mux.HandleFunc("GET /news", playH.NewsPage)
	mux.HandleFunc("GET /rank", playH.RankPage)
	mux.HandleFunc("GET /winner", playH.WinnerPage)

	mux.HandleFunc("GET /admin", adminH.AdminPage)
	mux.HandleFunc("POST /admin/login", adminH.AdminLogin)
	mux.HandleFunc("POST /admin/reset", adminH.ResetRound)
	mux.HandleFunc("POST /admin/area/advance", adminH.AdvanceForbiddenZones)
	mux.HandleFunc("POST /admin/user/delete", adminH.DeleteUser)
	mux.HandleFunc("GET /admin/backup", adminH.DownloadBackup)

	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.Dir(cfg.StaticDir))))

	rootHandler := handler.RecoverAndLog(handler.SessionMiddleware(store.DB, mux))

	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           rootHandler,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		log.Printf("Battle Royale Go server listening on :%s", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("server error: %v", err)
		}
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	log.Println("shutting down gracefully...")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("http shutdown error: %v", err)
	}
}
