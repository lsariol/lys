package dependencies

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/joho/godotenv"
	"github.com/lsariol/coveclient"
	"github.com/lsariol/lys/internal/config"
	"github.com/lsariol/lys/internal/database"
	"github.com/lsariol/lys/internal/runtime/settings"
)

type Deps struct {
	Config   *config.Config
	Settings *settings.Store
	HTTP     *http.Client
	Logger   string
	Cove     *coveclient.Client
	DB       *database.Database
	CTX      context.Context
	BootTime time.Time
}

func New(ctx context.Context) *Deps {
	return &Deps{
		CTX: ctx,
	}
}

func (d *Deps) Initilize() error {

	cfg := config.New()

	_ = godotenv.Load(".env")

	cove := coveclient.New(os.Getenv("COVE_URL"), os.Getenv("COVE_TOKEN"))

	err := cfg.Initilize(cove)
	if err != nil {
		return fmt.Errorf("initilize config: %w", err)
	}

	tr := &http.Transport{
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 10,
		IdleConnTimeout:     90 * time.Second,
	}

	client := &http.Client{
		Transport: tr,
		Timeout:   10 * time.Second,
	}

	d.Cove = cove
	d.HTTP = client
	d.Config = cfg
	d.Logger = "*zap.Logger"
	d.DB = database.NewDatabase(cfg.Database)
	d.BootTime = time.Now()

	return nil
}
