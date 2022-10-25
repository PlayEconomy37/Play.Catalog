package main

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/PlayEconomy37/Play.Catalog/internal/data"
	"github.com/PlayEconomy37/Play.Common/database"
	"github.com/PlayEconomy37/Play.Common/filters"
	"github.com/PlayEconomy37/Play.Common/types"
	"github.com/PlayEconomy37/Play.Common/validator"
	"github.com/go-chi/chi/v5"
	"go.mongodb.org/mongo-driver/bson"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
)

// healthCheckHandler is the handler for the "GET /healthcheck" endpoint
func (app *Application) healthCheckHandler(w http.ResponseWriter, r *http.Request) {
	env := types.Envelope{
		"status": "available",
	}

	err := app.WriteJSON(w, http.StatusOK, env, nil)
	if err != nil {
		app.ServerErrorResponse(w, r, err)
	}
}

// getItemsHandler is the handler for the "GET /items" endpoint
func (app *Application) getItemsHandler(w http.ResponseWriter, r *http.Request) {
	// Create trace for the handler
	ctx, span := app.Tracer.Start(r.Context(), "Retrieving items")
	defer span.End()

	// Anonymous struct used to hold the expected values from the request's query string
	var input struct {
		Name     string
		MinPrice float64
		MaxPrice float64
		filters.Filters
	}

	// Read query string
	queryString := r.URL.Query()

	// Instantiate validator
	v := validator.New()

	// Extract values from query string if they exist
	input.Name = app.ReadStringFromQueryString(queryString, "name", "")
	input.MinPrice = app.ReadFloatFromQueryString(queryString, "min_price", database.DefaultPrice, v)
	input.MaxPrice = app.ReadFloatFromQueryString(queryString, "max_price", database.DefaultPrice, v)
	input.Filters.Page = app.ReadIntFromQueryString(queryString, "page", 1, v)
	input.Filters.PageSize = app.ReadIntFromQueryString(queryString, "page_size", 20, v)
	input.Filters.Sort = app.ReadStringFromQueryString(queryString, "sort", "_id")

	// Add the supported sort values for this endpoint to the sort safelist
	input.Filters.SortSafelist = []string{"_id", "name", "price", "-_id", "-name", "-price"}

	// Validate query string
	v.Check(validator.Between(input.MinPrice, 0.1, 1000), "min_price", "must be greater or equal to 0.1 or lower and equal to 1000")
	v.Check(validator.Between(input.MaxPrice, 0.1, 1000), "max_price", "must be greater or equal to 0.1 or lower and equal to 1000")

	// Only run this check if both min_price and max_price have been set
	if input.MinPrice != database.DefaultPrice && input.MaxPrice != database.DefaultPrice {
		v.Check(input.MaxPrice >= input.MinPrice, "max_price", "must be greater or equal to specified min_price")
	}

	filters.ValidateFilters(v, input.Filters)

	// Check the Validator instance for any errors
	if v.HasErrors() {
		span.SetStatus(codes.Error, "Validation failed")
		app.FailedValidationResponse(w, r, v.Errors)
		return
	}

	// Set query filters
	filter := bson.M{}

	if input.Name != "" {
		filter["$text"] = bson.M{"$search": input.Name}
	}

	if input.MinPrice != database.DefaultPrice && input.MaxPrice == database.DefaultPrice {
		filter["price"] = bson.M{"$gte": input.MinPrice}
	} else if input.MaxPrice != database.DefaultPrice && input.MinPrice == database.DefaultPrice {
		filter["price"] = bson.M{"$lte": input.MaxPrice}
	} else if input.MaxPrice != database.DefaultPrice && input.MinPrice != database.DefaultPrice {
		filter["price"] = bson.M{"$gte": input.MinPrice, "$lte": input.MaxPrice}
	}

	// Retrieve all items
	items, metadata, err := app.ItemsRepository.GetAll(ctx, filter, input.Filters)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		app.ServerErrorResponse(w, r, err)
		return
	}

	env := types.Envelope{
		"items":    items,
		"metadata": metadata,
	}

	// Send back response
	err = app.WriteJSON(w, http.StatusOK, env, nil)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		app.ServerErrorResponse(w, r, err)
	}
}

// getItemHandler is the handler for the "GET /items/:id" endpoint
func (app *Application) getItemHandler(w http.ResponseWriter, r *http.Request) {
	// Create trace for the handler
	ctx, span := app.Tracer.Start(r.Context(), "Retrieving item")
	defer span.End()

	// Extract id parameter from request URL parameters
	id, err := app.ReadObjectIDParam(r)
	if err != nil {
		// Record error in the trace
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		span.SetAttributes(attribute.String("id", chi.URLParamFromCtx(r.Context(), "id")))

		// Throw Not found error if extracted id is not a valid ObjectID
		app.NotFoundResponse(w, r)
		return
	}

	// Record item id in the trace
	span.SetAttributes(attribute.String("id", id.Hex()))

	// Retrieve item with given id
	item, err := app.ItemsRepository.GetByID(ctx, id)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())

		switch {
		case errors.Is(err, database.ErrRecordNotFound):
			app.NotFoundResponse(w, r)
		default:
			app.ServerErrorResponse(w, r, err)
		}

		return
	}

	env := types.Envelope{
		"item": item,
	}

	err = app.WriteJSON(w, http.StatusOK, env, nil)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		app.ServerErrorResponse(w, r, err)
	}
}

// createItemHandler is the handler for the "POST /items" endpoint
func (app *Application) createItemHandler(w http.ResponseWriter, r *http.Request) {
	// Create trace for the handler
	ctx, span := app.Tracer.Start(r.Context(), "Creating item")
	defer span.End()

	// Declare an anonymous struct to hold the information that we expect to be in the
	// request body. This struct will be our *target decode destination*
	var input struct {
		Name        string  `json:"name"`
		Description string  `json:"description"`
		Price       float64 `json:"price"`
	}

	// Read request body and decode it into the input struct
	err := app.ReadJSON(w, r, &input)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		app.BadRequestResponse(w, r, err)
		return
	}

	// Copy the values from the input struct to a new Item struct
	item := data.Item{
		Name:        input.Name,
		Description: input.Description,
		Price:       input.Price,
		Version:     1,
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}

	// Initialize a new Validator instance
	v := validator.New()

	// Perform validation checks
	data.ValidateItem(v, item)

	if v.HasErrors() {
		span.SetStatus(codes.Error, "Validation failed")
		app.FailedValidationResponse(w, r, v.Errors)
		return
	}

	// Record item attributes in trace
	span.SetAttributes(
		attribute.String("name", item.Name),
		attribute.String("description", item.Description),
		attribute.Float64("price", item.Price),
	)

	// Create a record in the database
	id, err := app.ItemsRepository.Create(ctx, item)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		app.ServerErrorResponse(w, r, err)
		return
	}

	// When sending a HTTP response, we want to include a Location header to let the
	// client know which URL they can find the newly-created resource at
	headers := make(http.Header)
	headers.Set("Location", fmt.Sprintf("/items/%s", id.Hex()))

	env := types.Envelope{
		"message": "Item created successfully",
	}

	err = app.WriteJSON(w, http.StatusCreated, env, headers)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		app.ServerErrorResponse(w, r, err)
	}
}

// updateItemHandler is the handler for the "PUT /items/:id" endpoint
func (app *Application) updateItemHandler(w http.ResponseWriter, r *http.Request) {
	// Create trace for the handler
	ctx, span := app.Tracer.Start(r.Context(), "Updating item")
	defer span.End()

	// Extract id parameter from request URL parameters
	id, err := app.ReadObjectIDParam(r)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		span.SetAttributes(attribute.String("id", chi.URLParamFromCtx(r.Context(), "id")))

		// Throw Not found error if extracted id is not a valid ObjectID
		app.NotFoundResponse(w, r)
		return
	}

	// Record item id in the trace
	span.SetAttributes(attribute.String("id", id.Hex()))

	// Retrieve item with given id
	item, err := app.ItemsRepository.GetByID(ctx, id)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())

		switch {
		case errors.Is(err, database.ErrRecordNotFound):
			app.NotFoundResponse(w, r)
		default:
			app.ServerErrorResponse(w, r, err)
		}

		return
	}

	// We use pointers so that we get a nil value when decoding these values from JSON.
	// This way we can check if a user has provided the key/value pair in the JSON or not.
	var input struct {
		Name        *string  `json:"name"`
		Description *string  `json:"description"`
		Price       *float64 `json:"price"`
	}

	// Read request body and decode it into the input struct
	err = app.ReadJSON(w, r, &input)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		app.BadRequestResponse(w, r, err)
		return
	}

	// Copy the values from the input struct to the fetched item if they exist
	if input.Name != nil {
		item.Name = *input.Name
	}

	if input.Description != nil {
		item.Description = *input.Description
	}

	if input.Price != nil {
		item.Price = *input.Price
	}

	// Update item's updated at date
	item.UpdatedAt = time.Now().UTC()

	// Initialize a new Validator instance
	v := validator.New()

	// Perform validation checks
	data.ValidateItem(v, item)

	if v.HasErrors() {
		span.SetStatus(codes.Error, "Validation failed")
		app.FailedValidationResponse(w, r, v.Errors)
		return
	}

	// Update item in the database
	err = app.ItemsRepository.Update(ctx, item)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())

		switch {
		case errors.Is(err, database.ErrEditConflict):
			app.EditConflictResponse(w, r)
		default:
			app.ServerErrorResponse(w, r, err)
		}

		return
	}

	env := types.Envelope{
		"message": "Item updated successfully",
	}

	err = app.WriteJSON(w, http.StatusOK, env, nil)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		app.ServerErrorResponse(w, r, err)
	}
}

// deleteItemHandler is the handler for the "DELETE /items/:id" endpoint
func (app *Application) deleteItemHandler(w http.ResponseWriter, r *http.Request) {
	// Create trace for the handler
	ctx, span := app.Tracer.Start(r.Context(), "Deleting item")
	defer span.End()

	// Extract id parameter from request URL parameters
	id, err := app.ReadObjectIDParam(r)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		span.SetAttributes(attribute.String("id", chi.URLParamFromCtx(r.Context(), "id")))

		// Throw Not found error if extracted id is not a valid ObjectID
		app.NotFoundResponse(w, r)
		return
	}

	// Record item id in the trace
	span.SetAttributes(attribute.String("id", id.Hex()))

	// Delete item in the database
	err = app.ItemsRepository.Delete(ctx, id)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())

		switch {
		case errors.Is(err, database.ErrRecordNotFound):
			app.NotFoundResponse(w, r)
		default:
			app.ServerErrorResponse(w, r, err)
		}

		return
	}

	env := types.Envelope{
		"message": "Item deleted successfully",
	}

	err = app.WriteJSON(w, http.StatusOK, env, nil)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		app.ServerErrorResponse(w, r, err)
	}
}
