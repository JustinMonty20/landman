package soil

import (
	"time"

	"github.com/JustinMonty20/landman/internal/county"
	"github.com/twpayne/go-geom"
)

// Polygon is one county soil feature, independent of any parcel or database id.
type Polygon struct {
	County         county.ID
	SourceName     string
	ObjectID       string
	MapUnitKey     string
	SpatialVersion *float64
	SurveySavedOn  *time.Time
	Shape          *geom.T
}
