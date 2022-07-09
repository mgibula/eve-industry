package server

import (
	"fmt"
	"log"
	"path/filepath"
	"strings"

	"github.com/gin-contrib/multitemplate"
	"github.com/gin-gonic/gin"
	"github.com/wader/gormstore/v2"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/mgibula/eve-industry/server/calculator"
	"github.com/mgibula/eve-industry/server/config"
	"github.com/mgibula/eve-industry/server/db"
	"github.com/mgibula/eve-industry/server/layout"
	"github.com/mgibula/eve-industry/server/locations"
	"github.com/mgibula/eve-industry/server/sessions"
	"github.com/mgibula/eve-industry/server/sso"
)

type Server struct {
	gin *gin.Engine
}

func loadTemplates() multitemplate.Renderer {
	r := multitemplate.NewRenderer()

	layouts, err := filepath.Glob("resources/*.layout.tmpl")
	if err != nil {
		panic(err.Error())
	}

	includes, err := filepath.Glob("resources/views/*.tmpl")
	if err != nil {
		panic(err.Error())
	}

	for _, layout := range layouts {
		for _, include := range includes {
			files := make([]string, 2)
			files[0] = layout
			files[1] = include

			name := fmt.Sprintf("%s/%s", filepath.Base(strings.TrimSuffix(layout, ".layout.tmpl")), filepath.Base(include))
			r.AddFromFiles(name, files...)
			log.Println(name)
		}
	}

	return r
}

func Auth() gin.HandlerFunc {
	return func(c *gin.Context) {
		session := sessions.OpenSession(c)

		logged, exists := session.Get("current_user").(db.ESIUser)
		if exists {
			// Get current version from DB
			evedb := db.OpenEveDatabase()

			current := db.ESIUser{}
			err := evedb.Take(&current, logged.ID).Error
			if err == nil {
				c.Set("user", current)
			}
		}

		c.Next()
	}
}

func CreateServer() Server {
	if *config.ClientId == "" || *config.SecretKey == "" {
		log.Fatalln("Both -client-id and -secret-key parameters are required")
	}

	db.InitEveDatabase()

	db, err := gorm.Open(sqlite.Open("resources/sessions.db"), &gorm.Config{})
	if err != nil {
		panic(err)
	}

	sessiondb := gormstore.NewOptions(db, gormstore.Options{}, []byte("secret-hash-key"), nil)
	sessiondb.MaxLength(0)

	result := Server{}
	result.gin = gin.Default()
	result.gin.HTMLRender = loadTemplates()
	result.gin.Use(sessions.Middleware(sessiondb))
	result.gin.Use(Auth())
	result.gin.Static("/static", "resources/public")
	result.gin.Static("/webfonts", "resources/public/webfonts")
	result.gin.StaticFile("/favicon.ico", "resources/public/favicon.ico")

	calculator.RegisterRoutes(result.gin)
	sso.RegisterRoutes(result.gin)
	locations.RegisterRoutes(result.gin)
	result.gin.GET("/dashboard", IndexController)
	result.gin.GET("/", IndexController)

	return result
}

func (s *Server) Run(listen string) {
	s.gin.Run(listen)
}

func IndexController(c *gin.Context) {
	_, exists := c.Get("user")
	if exists {
		// esi := esi.NewESIClient(db.OpenEveDatabase(), maybe_user.(db.ESIUser))
		// esi.ListSkills()
	}

	layout.Render(c, "default/login.tmpl", gin.H{})
}
