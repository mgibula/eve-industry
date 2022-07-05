package calculator

import (
	"encoding/gob"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/mgibula/eve-industry/server/db"
	"github.com/mgibula/eve-industry/server/layout"
	"github.com/mgibula/eve-industry/server/sessions"
)

type productionPlan struct {
	Blueprint db.EVEBlueprint
	Runs      uint32
	ME        uint32
	PE        uint32
	Selected  bool
	Decryptor uint64
	Jobs      uint32
}

type productionPlans struct {
	Plans []productionPlan
}

func (l *productionPlans) addBlueprint(blueprint db.EVEBlueprint, runs uint32, me uint32, pe uint32, selected bool) {
	exists := false

	for i, existing := range l.Plans {
		if existing.Blueprint.ID == blueprint.ID {
			l.Plans[i].Selected = true
			exists = true
		} else {
			l.Plans[i].Selected = false
		}
	}

	if !exists {
		l.Plans = append(l.Plans, productionPlan{
			Blueprint: blueprint,
			Runs:      runs,
			ME:        me,
			PE:        pe,
			Selected:  selected,
			Jobs:      1,
		})
	}
}

func (l *productionPlans) getSelectedPlan() *productionPlan {
	for i, existing := range l.Plans {
		if existing.Selected {
			return &l.Plans[i]
		}
	}

	return nil
}

func (l *productionPlans) save(c *gin.Context) {
	session := sessions.OpenSession(c)
	session.Set("primary_blueprints", *l)
}

func getProductionPlans(c *gin.Context) productionPlans {
	session := sessions.OpenSession(c)

	maybe_list, exists := session.Get("primary_blueprints").(productionPlans)
	if !exists {
		maybe_list = productionPlans{}
		maybe_list.Plans = make([]productionPlan, 0)

		session.Set("primary_blueprints", maybe_list)
	}

	return maybe_list
}

func RegisterRoutes(c *gin.Engine) {
	gob.Register(productionPlan{})
	gob.Register([]productionPlan{})
	gob.Register(productionPlans{})
	gob.Register([]productionPlans{})

	c.GET("/production/calculator", indexHandler)
	c.GET("/production/list-blueprints", listBlueprintsHandler)
	c.POST("/production/calculator/add-blueprint", addBlueprintHandler)
	c.GET("/production/calculator/render-blueprint-card", renderBlueprintCard)
	c.GET("/production/calculator/render-blueprint-list", renderBlueprintList)
}

func indexHandler(c *gin.Context) {
	layout.Render(c, "default/calculator.tmpl", gin.H{})
}

func listBlueprintsHandler(c *gin.Context) {
	phrase := c.Query("q")

	var blueprints []db.EVEBlueprint

	db := db.OpenEveDatabase()
	db.Where("name like ?", phrase+"%").Find(&blueprints)

	result := make([]string, len(blueprints))
	for i, blueprint := range blueprints {
		result[i] = blueprint.Name
	}

	c.JSON(http.StatusOK, result)
}

func addBlueprintHandler(c *gin.Context) {
	type params struct {
		BlueprintName string `form:"blueprint_name"`
	}

	var form params
	c.Bind(&form)

	evedb := db.OpenEveDatabase()
	var blueprint db.EVEBlueprint
	err := evedb.Where("name = ?", form.BlueprintName).Take(&blueprint).Error
	if err != nil {
		c.AbortWithStatus(http.StatusNotFound)
		return
	}

	blueprints := getProductionPlans(c)
	blueprints.addBlueprint(blueprint, 1, blueprint.GetDefaultME(), blueprint.GetDefaultPE(), true)
	blueprints.save(c)
}

func renderBlueprintCard(c *gin.Context) {
	plans := getProductionPlans(c)
	selectedPlan := plans.getSelectedPlan()

	if selectedPlan != nil {
		layout.Render(c, "ajax/blueprint-card.tmpl", gin.H{
			"plan": selectedPlan,
		})
	}
}

func renderBlueprintList(c *gin.Context) {
	plans := getProductionPlans(c)

	layout.Render(c, "ajax/blueprint-list.tmpl", gin.H{
		"production": plans,
	})
}
