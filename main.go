package main

import (
	"encoding/json"
	"github.com/gin-contrib/cors"
	"github.com/gin-contrib/static"
	"github.com/gin-gonic/gin"
	"github.com/mattn/go-colorable"
	"github.com/rs/xid"
	"github.com/subosito/gotenv"
	"gitlab.com/milan44/logger"
	"io/ioutil"
	"math/rand"
	"os"
	"os/signal"
	"sync"
	"time"
)

var (
	ginLogger gin.HandlerFunc
	log       logger.ShortLogger

	vehicleMap      map[string]string
	vehicleMapMutex sync.Mutex

	oneTimeTokens     = make(map[string]time.Time)
	oneTimeTokenMutex sync.Mutex
)

func main() {
	_ = os.Setenv("TZ", "UTC")

	log = logger.NewGinStyleLogger(false)

	err := gotenv.Load(".env")
	if err != nil {
		log.Error("Failed to load .env")
		return
	}

	err = loadJSON("vehicle-map.json", &vehicleMap)
	if err != nil {
		log.Error("Failed to load vehicle-map.json")
		log.ErrorE(err)
		return
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
		if !checkSession(c, false) {
			log.Info("Rejected unauthorized login")
			return
		}

		handleSocket(c.Writer, c.Request, c)
	})

	r.GET("/map/go/token", func(c *gin.Context) {
		if !checkSession(c, true) {
			log.Info("Rejected unauthorized login")
			return
		}

		token := xid.New().String()

		oneTimeTokenMutex.Lock()
		oneTimeTokens[token] = time.Now()
		oneTimeTokenMutex.Unlock()

		c.JSON(200, map[string]interface{}{
			"status": true,
			"token":  token,
		})
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

func loadJSON(file string, dst *map[string]string) error {
	b, err := ioutil.ReadFile(file)
	if err != nil {
		return err
	}

	return json.Unmarshal(b, dst)
}
