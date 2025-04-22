package gis

import (
  "context"
  "github.com/JustinMonty20/landman/internal/models"
)

/*
  first iteration here
  each county connector that implements this will end having their own tim  ing frequency to update. Information like that doesn't need to go there   right now.
  
  Each county has its own diffing logic and then from there it will transf  orm the information back into our simplified types.
*/
type Connector interface {
  GetData(ctx context.Context) ([]models.Parcel, error)
  Diff(ctx context.Context) ([]models.Parcel, error)
}
