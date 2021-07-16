package main

import (
	"encoding/json"
	"github.com/gin-contrib/cors"
	"github.com/gin-contrib/static"
	"github.com/gin-gonic/gin"
	"github.com/mattn/go-colorable"
	"github.com/subosito/gotenv"
	"gitlab.com/milan44/logger"
	"io/ioutil"
	"math/rand"
	"os"
	"os/exec"
	"os/signal"
	"time"
)

var (
	ginLogger gin.HandlerFunc
	log       logger.ShortLogger
)

func main() {
	_ = os.Setenv("TZ", "UTC")

	log = logger.NewGinStyleLogger(false)

	err := gotenv.Load(".env")
	if err != nil {
		log.Error("Failed to load .env")
		return
	}

	if _, err := os.Stat("./tiles"); os.IsNotExist(err) {
		_ = os.MkdirAll("./tiles", 0777)

		log.Info("Extracting tiles... (This may take a while)")
		err := exec.Command("tar", "-xvzf", "tiles.tar.gz", "-C", "tiles").Run()
		if err != nil {
			log.Warning("Failed to extract tiles")
		}
	}

	rand.Seed(time.Now().UnixNano())

	go func() {
		sigc := make(chan os.Signal, 1)
		signal.Notify(sigc,
			os.Interrupt,
		)

		<-sigc

		log.Warning("Caught interrupt")

		os.Exit(0)
	}()

	b, err := ioutil.ReadFile("afk.json")
	if err == nil {
		_ = json.Unmarshal(b, &lastPosition)
	}

	gin.DefaultWriter = colorable.NewColorableStdout()
	gin.ForceConsoleColor()
	gin.SetMode(gin.ReleaseMode)

	r := gin.New()

	r.Use(gin.Recovery())
	r.Use(cors.Default())
	ginLogger = logger.GinLoggerMiddleware()

	r.Use(static.Serve("/map/go/tiles", static.LocalFile("./tiles", false)))

	r.GET("/map/go/socket", func(c *gin.Context) {
		if !checkSession(c) {
			log.Info("Rejected unauthorized login")
			return
		}

		handleSocket(c.Writer, c.Request, c)
	})

	r.POST("/map/go/history", handleHistory)

	go startDataLoop()

	log.Info("Starting server on port 8443")

	cert := os.Getenv("SSL_CERT")
	key := os.Getenv("SSL_KEY")

	err = r.RunTLS(":8443", cert, key)
	if err != nil {
		log.Warning("Failed to start TLS server (invalid SSL_CERT or SSL_KEY)")
		log.Info("Starting non-TLS server on port 8080")

		err = r.Run(":8080")
		if err != nil {
			panic(err)
		}
	}
}
