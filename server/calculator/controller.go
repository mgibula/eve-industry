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
	Runs      int64 // runs requested by user
	ME        int32
	PE        int32
	Selected  bool
	Decryptor uint64

	// Filled for templates
	TotalQuantity  int64
	TotalRuns      int64
	AdditionalRuns int64
	Buildable      int64
	Built          int64
	Jobs           []int64
}

type productionPlans struct {
	Plans []*productionPlan
}

func (l *productionPlans) addBlueprint(blueprint db.EVEBlueprint, runs int64, me int32, pe int32, selected bool) {
	exists := false

	for _, existing := range l.Plans {
		if existing.Blueprint.ID == blueprint.ID {
			exists = true
		}
	}

	if !exists {
		l.Plans = append(l.Plans, &productionPlan{
			Blueprint: blueprint,
			Runs:      runs,
			ME:        me,
			PE:        pe,
			Selected:  false,
		})
	}

	if selected {
		l.markPlanSelected(blueprint.ID)
	}
}

func (l *productionPlans) removeBlueprint(blueprintID uint64) {
	newPlans := make([]*productionPlan, 0)

	for _, plan := range l.Plans {
		if plan.Blueprint.ID != blueprintID {
			newPlans = append(newPlans, plan)
		}
	}

	l.Plans = newPlans
}

func (l *productionPlans) markPlanSelected(blueprintID uint64) bool {
	exists := false

	for i, existing := range l.Plans {
		if existing.Blueprint.ID == blueprintID {
			l.Plans[i].Selected = true
			exists = true
		} else {
			l.Plans[i].Selected = false
		}
	}

	return exists
}

func (l *productionPlans) getSelectedPlan() *productionPlan {
	for i, existing := range l.Plans {
		if existing.Selected {
			return l.Plans[i]
		}
	}

	return nil
}

func (l *productionPlans) purgeSecondaryBlueprints() {
	calculator := NewMaterialCalculator()
	for _, plan := range l.Plans {
		calculator.AddBlueprintSettings(plan.Blueprint.ID, plan.ME, plan.PE, plan.Decryptor)
	}

	for _, plan := range l.Plans {
		calculator.AddQuantity(plan.Blueprint.ManufacturingProductId, plan.Blueprint.ManufacturingProductName, plan.Blueprint.ManufacturingProductOutputQuantity*plan.Runs, true)
	}

	newPlans := make([]*productionPlan, 0)

	materials := calculator.GetAllMaterials()

	for _, plan := range l.Plans {
		if getMaterialInfo(plan.Blueprint.ManufacturingProductId, materials).BuildInfo.Runs > 0 {
			newPlans = append(newPlans, plan)
		}
	}

	l.Plans = newPlans
}

func (l *productionPlans) save(c *gin.Context) {
	session := sessions.OpenSession(c)
	session.Set("primary_blueprints", *l)
	session.Save()
}

func getProductionPlans(c *gin.Context) productionPlans {
	session := sessions.OpenSession(c)

	maybe_list, exists := session.Get("primary_blueprints").(productionPlans)
	if !exists {
		maybe_list = productionPlans{}
		maybe_list.Plans = make([]*productionPlan, 0)

		session.Set("primary_blueprints", maybe_list)
		session.Save()
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
	c.POST("/production/calculator/render-blueprint-card", renderBlueprintCard)
	c.GET("/production/calculator/render-blueprint-list", renderBlueprintList)
	c.GET("/production/calculator/change-blueprint-settings", changeBlueprintSettingsHandler)
	c.GET("/production/calculator/add-secondary-blueprint", addSecondaryBlueprintHandler)
	c.GET("/production/calculator/remove-secondary-blueprint", removeSecondaryBlueprintHandler)
	c.POST("/production/calculator/remove-blueprint", removeBlueprintHandler)
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

func changeBlueprintSettingsHandler(c *gin.Context) {
	type params struct {
		ME        int32  `form:"me" binding:"-"`
		PE        int32  `form:"pe" binding:"-"`
		Runs      int64  `form:"runs" binding:"-"`
		Decryptor uint64 `form:"decryptor" binding:"-"`
	}

	var form params
	c.Bind(&form)

	plans := getProductionPlans(c)
	selectedPlan := plans.getSelectedPlan()
	if selectedPlan == nil {
		c.AbortWithStatus(http.StatusNotFound)
		return
	}

	if form.ME > 10 {
		form.ME = 10
	} else if form.ME < 0 {
		form.ME = 0
	}

	if form.PE > 20 {
		form.PE = 20
	} else if form.PE < 0 {
		form.PE = 0
	}

	selectedPlan.ME = form.ME
	selectedPlan.PE = form.PE
	selectedPlan.Runs = form.Runs
	selectedPlan.Decryptor = form.Decryptor

	if selectedPlan.Decryptor > 0 {
		evedb := db.OpenEveDatabase()
		var decryptor db.EVEDecryptor
		err := evedb.Where("id = ?", selectedPlan.Decryptor).Take(&decryptor).Error
		if err != nil {
			c.AbortWithStatus(http.StatusNotFound)
			return
		}

		selectedPlan.ME += decryptor.MEModifier
		selectedPlan.PE += decryptor.PEModifier
	}

	plans.purgeSecondaryBlueprints()
	plans.save(c)
}

func addSecondaryBlueprintHandler(c *gin.Context) {
	type params struct {
		BlueprintID uint64 `form:"blueprint_id"`
	}

	var form params
	c.Bind(&form)

	evedb := db.OpenEveDatabase()
	var blueprint db.EVEBlueprint
	err := evedb.Where("id = ?", form.BlueprintID).Take(&blueprint).Error
	if err != nil {
		c.AbortWithStatus(http.StatusNotFound)
		return
	}

	plans := getProductionPlans(c)
	plans.addBlueprint(blueprint, 0, blueprint.GetDefaultME(), blueprint.GetDefaultPE(), false)
	plans.save(c)

	renderBlueprintList(c)
}

func removeBlueprintHandler(c *gin.Context) {
	type params struct {
		BlueprintID uint64 `form:"blueprint_id"`
	}

	var form params
	c.Bind(&form)

	plans := getProductionPlans(c)
	plans.removeBlueprint(form.BlueprintID)
	plans.purgeSecondaryBlueprints()
	plans.save(c)

	renderBlueprintList(c)
}

func removeSecondaryBlueprintHandler(c *gin.Context) {
	type params struct {
		BlueprintID uint64 `form:"blueprint_id"`
	}

	var form params
	c.Bind(&form)

	plans := getProductionPlans(c)
	plans.removeBlueprint(form.BlueprintID)
	plans.purgeSecondaryBlueprints()
	plans.save(c)

	renderBlueprintList(c)
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

	renderBlueprintList(c)
}

func renderBlueprintCard(c *gin.Context) {
	type params struct {
		BlueprintID uint64 `form:"blueprint_id" binding:"-"`
	}

	var form params
	c.Bind(&form)

	plans := getProductionPlans(c)
	if form.BlueprintID > 0 {
		plans.markPlanSelected(form.BlueprintID)
		plans.save(c)
	}

	selectedPlan := plans.getSelectedPlan()
	if selectedPlan == nil {
		return
	}

	calculator := NewMaterialCalculator()
	for _, plan := range plans.Plans {
		calculator.AddBlueprintSettings(plan.Blueprint.ID, plan.ME, plan.PE, plan.Decryptor)
	}

	for _, plan := range plans.Plans {
		calculator.AddQuantity(plan.Blueprint.ManufacturingProductId, plan.Blueprint.ManufacturingProductName, plan.Blueprint.ManufacturingProductOutputQuantity*plan.Runs, true)
	}

	materials := calculator.GetAllMaterials()

	for _, plan := range plans.Plans {
		info := getMaterialInfo(plan.Blueprint.ManufacturingProductId, materials)
		plan.Jobs = info.BuildInfo.Jobs
		plan.TotalRuns = info.BuildInfo.Runs
		plan.AdditionalRuns = info.BuildInfo.Runs - plan.Runs
		plan.TotalQuantity = info.BuildInfo.Runs * plan.Blueprint.ManufacturingProductOutputQuantity
	}

	evedb := db.OpenEveDatabase()
	var decryptors []db.EVEDecryptor
	evedb.Find(&decryptors)

	type ProductInfo struct {
		ProductID   uint64
		ProductName string
		Quantity    int64
	}

	products := make([]ProductInfo, 0)
	for _, plan := range plans.Plans {
		if plan.Runs == 0 {
			continue
		}

		info := ProductInfo{
			ProductID:   plan.Blueprint.ManufacturingProductId,
			ProductName: plan.Blueprint.ManufacturingProductName,
			Quantity:    plan.Runs * plan.Blueprint.ManufacturingProductOutputQuantity,
		}

		products = append(products, info)
	}

	layout.Render(c, "ajax/blueprint-card.tmpl", gin.H{
		"plan":       selectedPlan,
		"decryptors": decryptors,
		"materials":  calculator.GetMaterialsFor(selectedPlan.Blueprint.ManufacturingProductId),
		"products":   products,
		"excess":     calculator.GetAllMaterials(),
	})
}

func renderBlueprintList(c *gin.Context) {
	plans := getProductionPlans(c)

	calculator := NewMaterialCalculator()
	for _, plan := range plans.Plans {
		calculator.AddBlueprintSettings(plan.Blueprint.ID, plan.ME, plan.PE, plan.Decryptor)
	}

	for _, plan := range plans.Plans {
		calculator.AddQuantity(plan.Blueprint.ManufacturingProductId, plan.Blueprint.ManufacturingProductName, plan.Blueprint.ManufacturingProductOutputQuantity*plan.Runs, true)
	}

	materials := calculator.GetAllMaterials()

	for _, plan := range plans.Plans {
		info := getMaterialInfo(plan.Blueprint.ManufacturingProductId, materials)
		plan.TotalQuantity = info.Quantity
		plan.TotalRuns = info.BuildInfo.Runs
		plan.AdditionalRuns = info.BuildInfo.Runs - plan.Runs

		submaterials := calculator.GetMaterialsFor(plan.Blueprint.ManufacturingProductId)
		for _, submaterial := range submaterials {
			plan.Buildable++
			if submaterial.IsBuilt {
				plan.Built++
			}
		}
	}

	layout.Render(c, "ajax/blueprint-list.tmpl", gin.H{
		"production": plans,
		"materials":  calculator.GetAllMaterials(),
	})
}

func getMaterialInfo(materialID uint64, materials []MaterialInfoFull) MaterialInfoFull {
	for _, material := range materials {
		if material.MaterialID == materialID {
			return material
		}
	}

	return MaterialInfoFull{}
}
