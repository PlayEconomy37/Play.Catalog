package data

import (
	"context"
	"time"

	"github.com/PlayEconomy37/Play.Catalog/internal/constants"
	"github.com/PlayEconomy37/Play.Common/validator"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// Item is a struct that defines an item in our application
type Item struct {
	ID          primitive.ObjectID `json:"id" bson:"_id,omitempty"`
	Name        string             `json:"name" bson:"name"`
	Description string             `json:"description" bson:"description"`
	Price       float64            `json:"price" bson:"price"`
	Version     int32              `json:"version" bson:"version"`
	CreatedAt   time.Time          `json:"-" bson:"created_at"`
	UpdatedAt   time.Time          `json:"-" bson:"updated_at"`
}

// GetID returns the id of an item.
// This method is necessary for our generic constraint of our mongo repository.
func (i Item) GetID() primitive.ObjectID {
	return i.ID
}

// GetVersion returns the version of an item.
// This method is necessary for our generic constraint of our mongo repository.
func (i Item) GetVersion() int32 {
	return i.Version
}

// SetVersion sets the version of an item to the given value and returns the item.
// This method is necessary for our generic constraint of our mongo repository.
func (i Item) SetVersion(version int32) Item {
	i.Version = version

	return i
}

// ValidateItem runs validation checks on the `Item` struct
func ValidateItem(v *validator.Validator, item Item) {
	v.Check(item.Name != "", "name", "must be provided")
	v.Check(item.Description != "", "name", "must be provided")
	v.Check(validator.Between(item.Price, 0.1, 1000.0), "price", "must be greater or equal to 0.1 and lower or equal to 1000")
}

// CreateItemsCollection creates items collection in MongoDB database
func CreateItemsCollection(client *mongo.Client, databaseName string) error {
	db := client.Database(databaseName)

	// JSON validation schema
	jsonSchema := bson.M{
		"bsonType":             "object",
		"required":             []string{"name", "description", "price", "version", "created_at", "updated_at"},
		"additionalProperties": false,
		"properties": bson.M{
			"_id": bson.M{
				"bsonType":    "objectId",
				"description": "Document ID",
			},
			"name": bson.M{
				"bsonType":    "string",
				"description": "Name of the item",
			},
			"description": bson.M{
				"bsonType":    "string",
				"description": "Description of the item",
			},
			"price": bson.M{
				"bsonType":    "double",
				"minimum":     0.1,
				"description": "Price of the item",
			},
			"version": bson.M{
				"bsonType":    "int",
				"minimum":     1,
				"description": "Document version",
			},
			"created_at": bson.M{
				"bsonType":    "date",
				"description": "Creation date",
			},
			"updated_at": bson.M{
				"bsonType":    "date",
				"description": "Last update date",
			},
		},
	}

	validator := bson.M{
		"$jsonSchema": jsonSchema,
	}

	// Create collection
	opts := options.CreateCollection().SetValidator(validator)
	err := db.CreateCollection(context.Background(), constants.ItemsCollection, opts)
	if err != nil {
		// Returns error if collection already exists so we ignore it
		return nil
	}

	// Create unique and text indexes
	indexModels := []mongo.IndexModel{
		{
			Keys:    bson.M{"name": 1},
			Options: options.Index().SetUnique(true),
		},
		{
			Keys:    bson.M{"description": 1},
			Options: options.Index().SetUnique(true),
		},
		{
			Keys: bson.M{"name": "text"},
		},
	}

	_, err = db.Collection(constants.ItemsCollection).Indexes().CreateMany(context.Background(), indexModels)
	if err != nil {
		return err
	}

	return nil
}
