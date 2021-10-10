package main

import (
	"encoding/json"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/mattn/go-colorable"
	"github.com/rs/xid"
	"github.com/subosito/gotenv"
	"gitlab.com/milan44/logger"
	"io/ioutil"
	"math/rand"
	"os"
	"os/signal"
	"regexp"
	"strings"
	"sync"
	"time"
)

var (
	ginLogger gin.HandlerFunc
	log       logger.ShortLogger

	vehicleMap      map[string]string
	vehicleMapMutex sync.Mutex

	displayMap      map[string]string
	displayMapMutex sync.Mutex

	oneTimeTokens     = make(map[string]OTT)
	oneTimeTokenMutex sync.Mutex

	SessionDirectory string
)

type OTT struct {
	time    time.Time
	cluster string
}

func main() {
	_ = os.Setenv("TZ", "UTC")

	log = logger.NewGinStyleLogger(false)

	err := gotenv.Load(".env")
	if err != nil {
		log.Error("Failed to load .env")
		return
	}

	root := strings.TrimRight(os.Getenv("PanelRoot"), string(os.PathSeparator))
	SessionDirectory = root + "/storage/framework/session_storage"
	stat, err := os.Stat(SessionDirectory)
	if err != nil || !stat.IsDir() {
		log.Error("Failed to read PanelRoot '" + SessionDirectory + "'")
		return
	}

	log.Debug("Using '" + SessionDirectory + "' for sessions")

	err = loadJSON("vehicle-map.json", &vehicleMap)
	if err != nil {
		log.Error("Failed to load vehicle-map.json")
		log.ErrorE(err)
		return
	}

	err = loadJSON("display-map.json", &displayMap)
	if err != nil {
		log.Error("Failed to load display-map.json")
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

	_ = doHistoryCleanup()

	gin.DefaultWriter = colorable.NewColorableStdout()
	gin.ForceConsoleColor()
	gin.SetMode(gin.ReleaseMode)

	r := gin.New()

	corsConf := cors.DefaultConfig()
	corsConf.AllowWebSockets = true
	corsConf.AllowAllOrigins = true
	corsConf.AllowWildcard = true
	corsConf.AllowHeaders = append(corsConf.AllowHeaders, "x-requested-with")

	r.Use(gin.Recovery())
	r.Use(cors.New(corsConf))
	ginLogger = logger.GinLoggerMiddleware()

	r.GET("/socket", func(c *gin.Context) {
		if !checkSession(c, false) {
			log.Info("Rejected unauthorized login")
			return
		}

		handleSocket(c.Writer, c.Request, c, SocketTypeMap)
	})

	r.GET("/staff-chat", func(c *gin.Context) {
		if !checkSession(c, false) {
			log.Info("Rejected unauthorized login")
			return
		}

		handleSocket(c.Writer, c.Request, c, SocketTypeStaffChat)
	})

	r.GET("/token", func(c *gin.Context) {
		if !checkSession(c, true) {
			log.Info("Rejected unauthorized login")
			return
		}

		token := xid.New().String()

		oneTimeTokenMutex.Lock()
		oneTimeTokens[token] = OTT{
			time:    time.Now(),
			cluster: c.Query("cluster"),
		}
		oneTimeTokenMutex.Unlock()

		c.JSON(200, map[string]interface{}{
			"status": true,
			"token":  token,
		})
	})

	r.GET("/history/heatmap/:server/:day", func(c *gin.Context) {
		if !checkSession(c, true) {
			log.Info("Rejected unauthorized login")
			return
		}

		server := c.Param("server")
		day := c.Param("day")

		_, dayErr := time.Parse("2006-01-02", day)
		rgx := regexp.MustCompile(`(?m)^c\d+s\d+$`)
		if !rgx.MatchString(server) || dayErr != nil {
			c.JSON(200, map[string]interface{}{
				"status": false,
				"error":  "invalid server or day",
			})
			return
		}

		cache, err := generateHeatMapForDay(server, day)
		if err != nil {
			c.JSON(200, map[string]interface{}{
				"status": false,
				"error":  err.Error(),
			})
			return
		}

		c.File(cache)
	})

	go startDataLoop()
	go startDutyLoop()
	go startStaffChatLoop()

	cert := os.Getenv("SSL_CERT")
	key := os.Getenv("SSL_KEY")
	log.Info("Starting server on port 9999")

	err = r.RunTLS(":9999", cert, key)
	if err != nil {
		log.Warning("Failed to start TLS server (invalid SSL_CERT or SSL_KEY)")
		log.Info("Starting non-TLS server on port 8080")

		err = r.Run(":9999")
		if err != nil {
			panic(err)
		}
		panic(err)
	}
}

func loadJSON(file string, dst *map[string]string) error {
	b, err := ioutil.ReadFile(file)
	if err != nil {
		return err
	}

	return json.Unmarshal(b, dst)
}
