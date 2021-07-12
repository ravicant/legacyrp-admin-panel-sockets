package main

import (
	"github.com/gin-contrib/static"
	"github.com/gin-gonic/gin"
	"github.com/mattn/go-colorable"
	"github.com/subosito/gotenv"
	"gitlab.com/milan44/logger"
	"log"
	"math/rand"
	"os"
	"os/exec"
	"os/signal"
	"time"
)

func main() {
	_ = os.Setenv("TZ", "UTC")

	err := gotenv.Load(".env")
	if err != nil {
		log.Println("Failed to load .env")
		return
	}

	if _, err := os.Stat("./tiles"); os.IsNotExist(err) {
		_ = os.MkdirAll("./tiles", 0777)

		log.Println("Extracting tiles... (This may take a while)")
		err := exec.Command("tar", "-xvzf", "tiles.tar.gz", "-C", "tiles").Run()
		if err != nil {
			log.Println("Failed to extract tiles")
		}
	}

	rand.Seed(time.Now().UnixNano())

	go func() {
		sigc := make(chan os.Signal, 1)
		signal.Notify(sigc,
			os.Interrupt,
		)

		<-sigc

		log.Println("Caught interrupt")

		os.Exit(0)
	}()

	gin.DefaultWriter = colorable.NewColorableStdout()
	gin.ForceConsoleColor()
	gin.SetMode(gin.ReleaseMode)

	r := gin.New()

	r.Use(gin.Recovery())
	loggerWare := logger.GinLoggerMiddleware()

	r.Use(static.Serve("/map/go/tiles", static.LocalFile("./tiles", false)))

	r.GET("/map/go/socket", func(c *gin.Context) {
		loggerWare(c)

		handleSocket(c.Writer, c.Request, c)
	})

	go startDataLoop()

	log.Println("Starting server on port 8443")

	cert := os.Getenv("SSL_CERT")
	key := os.Getenv("SSL_KEY")

	err = r.RunTLS(":8443", cert, key)
	if err != nil {
		log.Println("Failed to start TLS server (invalid SSL_CERT or SSL_KEY)")
		log.Println("Starting non-TLS server on port 8080")

		err = r.Run(":8080")
		if err != nil {
			panic(err)
		}
	}
}
