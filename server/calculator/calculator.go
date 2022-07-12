package calculator

import (
	"math"
	"sort"

	"github.com/mgibula/eve-industry/server/db"
	"gorm.io/gorm"
)

type MaterialCalculator struct {
	EveDB             *gorm.DB
	BlueprintSettings map[uint64]BlueprintSettings
	Materials         map[uint64]*Material
}

type Material struct {
	parent               *MaterialCalculator
	BlueprintInfo        *db.EVEBlueprint
	MaterialID           uint64
	MaterialName         string
	RequestedQuantity    int64 // Requested by user
	TotalQuantity        int64 // Total quantity needed
	Submaterials         []db.EVEMaterial
	SubmaterialQuantites map[uint64]int64 // Submaterials needed for current item
	Jobs                 []int64          // Jobs needed, sliced by max runs per bpc
	Excess               int64
}

type MaterialInfo struct {
	MaterialID          uint64
	MaterialName        string
	Quantity            int64
	MaterialBlueprintID uint64
	IsBuilt             bool
}

type MaterialInfoFull struct {
	MaterialID          uint64
	MaterialName        string
	Quantity            int64
	Excess              int64
	MaterialBlueprintID uint64
	IsBuilt             bool

	MaterialBlueprintName string
	BuildInfo             struct {
		Runs int64
		Jobs []int64
		ME   int32
		PE   int32
	}
}

type BlueprintSettings struct {
	Blueprint db.EVEBlueprint
	ME        int32
	PE        int32
	Decryptor uint64
}

func NewMaterialCalculator() MaterialCalculator {
	result := MaterialCalculator{
		BlueprintSettings: make(map[uint64]BlueprintSettings),
		Materials:         make(map[uint64]*Material),
		EveDB:             db.OpenEveDatabase(),
	}

	return result
}

func (c *MaterialCalculator) AddBlueprintSettings(blueprintID uint64, me int32, pe int32, decryptor uint64) {
	blueprint := BlueprintSettings{
		ME:        me,
		PE:        pe,
		Decryptor: decryptor,
	}

	c.EveDB.Where("ID = ?", blueprintID).Take(&blueprint.Blueprint)
	c.BlueprintSettings[blueprintID] = blueprint
}

func (c *MaterialCalculator) AddQuantity(itemID uint64, name string, quantity int64, is_primary bool) {
	if _, exists := c.Materials[itemID]; !exists {
		material := Material{
			MaterialID:           itemID,
			MaterialName:         name,
			SubmaterialQuantites: make(map[uint64]int64),
			Jobs:                 make([]int64, 0),
			parent:               c,
		}

		var blueprint db.EVEBlueprint
		err := c.EveDB.Where("manufacturing_product_id = ?", itemID).Take(&blueprint).Error
		if err == nil {
			material.BlueprintInfo = &blueprint
			c.EveDB.Where("blueprint_id = ? and activity_id in (1, 11)", blueprint.ID).Find(&material.Submaterials)
		}

		c.Materials[itemID] = &material
	}

	material := c.Materials[itemID]
	material.addQuantity(quantity, is_primary)
}

func (c *MaterialCalculator) GetAllMaterials() []MaterialInfoFull {
	result := make([]MaterialInfoFull, 0)

	for _, requiredMaterial := range c.Materials {
		info := MaterialInfoFull{
			MaterialID:   requiredMaterial.MaterialID,
			MaterialName: requiredMaterial.MaterialName,
			Quantity:     requiredMaterial.TotalQuantity,
			Excess:       requiredMaterial.Excess,
		}

		if requiredMaterial.BlueprintInfo != nil {
			info.MaterialBlueprintID = requiredMaterial.BlueprintInfo.ID
			info.MaterialBlueprintName = requiredMaterial.BlueprintInfo.Name

			settings := c.getBlueprintSettings(requiredMaterial.BlueprintInfo.ID)
			if settings != nil {
				info.IsBuilt = true
				info.BuildInfo.Jobs = requiredMaterial.Jobs
				info.BuildInfo.Runs = requiredMaterial.getTotalRuns()
				info.BuildInfo.ME = settings.ME
				info.BuildInfo.PE = settings.PE
			}
		}

		result = append(result, info)
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].Quantity > result[j].Quantity
	})

	return result
}

func (c *MaterialCalculator) GetMaterialsFor(itemID uint64) []MaterialInfo {
	result := make([]MaterialInfo, 0)
	material := c.Materials[itemID]

	for _, submaterial := range material.Submaterials {
		info := MaterialInfo{
			MaterialID:          submaterial.MaterialId,
			MaterialName:        submaterial.MaterialName,
			MaterialBlueprintID: submaterial.MaterialBlueprintId,
			Quantity:            material.SubmaterialQuantites[submaterial.MaterialId],
			IsBuilt:             c.hasBlueprintSettings(submaterial.MaterialBlueprintId),
		}

		result = append(result, info)
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].Quantity > result[j].Quantity
	})

	return result
}

func (c *MaterialCalculator) getBlueprintSettings(blueprintID uint64) *BlueprintSettings {
	if value, exists := c.BlueprintSettings[blueprintID]; exists {
		return &value
	}

	return nil
}

func (c *MaterialCalculator) hasBlueprintSettings(blueprintID uint64) bool {
	_, exists := c.BlueprintSettings[blueprintID]
	return exists
}

func (material *Material) neededQuantity() int64 {
	return max(material.TotalQuantity, material.RequestedQuantity)
}

func (material *Material) neededJobs() []int64 {
	result := make([]int64, 0)
	quantityNeeded := material.neededQuantity()

	for quantityNeeded > 0 {
		runsRequired := max(1, int64(math.Ceil(float64(quantityNeeded)/float64(material.BlueprintInfo.ManufacturingProductOutputQuantity))))
		runsQueued := runsRequired

		// We assume T1 BPO and max runs BPC for others
		if !material.BlueprintInfo.IsTech1() {
			runsQueued = min(runsQueued, material.BlueprintInfo.ManufacturingMaxRuns)
		}

		quantityNeeded -= runsQueued * material.BlueprintInfo.ManufacturingProductOutputQuantity
		result = append(result, runsQueued)
	}

	return result
}

func (material *Material) getRequiredJobs() []int64 {
	return material.Jobs
}

func (material *Material) getTotalRuns() int64 {
	var result int64

	for _, runs := range material.Jobs {
		result += runs
	}

	return result
}

func (material *Material) addQuantity(quantity int64, is_primary bool) {
	material.TotalQuantity += quantity

	if is_primary {
		material.RequestedQuantity += quantity
	}

	if material.BlueprintInfo == nil {
		return
	}

	settings := material.parent.getBlueprintSettings(material.BlueprintInfo.ID)
	if settings == nil {
		return
	}

	material.Jobs = material.neededJobs()
	material.Excess = material.getTotalRuns()*material.BlueprintInfo.ManufacturingProductOutputQuantity - material.neededQuantity()

	for _, submaterial := range material.Submaterials {
		old_quantity := material.SubmaterialQuantites[submaterial.MaterialId]
		new_quantity := int64(0)

		for _, runs := range material.Jobs {
			new_quantity += max(int64(math.Ceil(float64(submaterial.Quantity)*float64(runs)*(1.0-float64(settings.ME)*0.01))), runs)
		}

		material.SubmaterialQuantites[submaterial.MaterialId] = new_quantity
		material.parent.AddQuantity(submaterial.MaterialId, submaterial.MaterialName, new_quantity-old_quantity, false)
	}
}

func max(x, y int64) int64 {
	if x > y {
		return x
	}
	return y
}

func min(x, y int64) int64 {
	if x > y {
		return y
	}
	return x
}
