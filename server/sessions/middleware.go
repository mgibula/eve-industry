package sessions

import (
	"log"

	"github.com/gin-gonic/gin"
	gorilla "github.com/gorilla/sessions"
	"github.com/wader/gormstore/v2"
)

type Session struct {
	store      *gormstore.Store
	session    *gorilla.Session
	ginContext *gin.Context
}

func (s *Session) Get(name string) any {
	return s.session.Values[name]
}

func (s *Session) Set(name string, value any) {
	s.session.Values[name] = value
}

func (s *Session) Delete(name string) {
	delete(s.session.Values, name)
}

func (s *Session) Save() {
	s.session.Save(s.ginContext.Request, s.ginContext.Writer)
}

const GinKey = "github.com/mgibula/eve-industry/server/sessions"

func OpenSession(c *gin.Context) *Session {
	session, _ := c.Get(GinKey)
	return session.(*Session)
}

func Middleware(store *gormstore.Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		handler := Session{}
		handler.store = store
		handler.ginContext = c

		session, err := store.Get(c.Request, "SESSID")
		if err != nil {
			log.Fatalln("Session store get", err)
		}

		handler.session = session

		c.Set(GinKey, &handler)
		c.Next()
	}
}
