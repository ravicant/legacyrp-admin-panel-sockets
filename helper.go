package main

import (
	"github.com/gin-gonic/gin"
	"os"
	"path/filepath"
	"regexp"
	"time"
)

func checkSession(c *gin.Context) bool {
	ginLogger(c)

	session, err := c.Cookie("legacy_rp_admin_v3_session_store")
	if err != nil || !validSession(session) {
		session := c.PostForm("token")

		if session == "" || !validSession(session) {
			c.Data(401, "text/plain", []byte("Unauthorized"))
			c.Abort()
			return false
		}
	}

	return true
}

func validSession(session string) bool {
	_ = os.MkdirAll("sessions", 0777)
	now := time.Now()
	_ = filepath.Walk("sessions", func(path string, info os.FileInfo, err error) error {
		if info != nil && now.Sub(info.ModTime()) > 1*time.Hour {
			_ = os.RemoveAll(path)
		}
		return nil
	})

	rgx := regexp.MustCompile(`(?mi)[^a-z0-9]`)
	session = rgx.ReplaceAllString(session, "")
	if session == "" {
		return false
	}

	sessionFile := "sessions/" + session + ".session"
	if _, err := os.Stat(sessionFile); err != nil {
		return false
	}

	return true
}
