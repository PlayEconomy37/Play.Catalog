package main

import (
	"net/http"

	"github.com/PlayEconomy37/Play.Catalog/assets"
	"github.com/go-chi/chi/v5"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/riandyrn/otelchi"
)

// routes defines all the routes and handlers in our application
func (app *Application) routes() http.Handler {
	router := chi.NewRouter()

	router.NotFound(http.HandlerFunc(app.NotFoundResponse))
	router.MethodNotAllowed(http.HandlerFunc(app.MethodNotAllowedResponse))

	router.Use(app.RecoverPanic)
	// router.Use(app.HTTPMetrics(app.Config.ServiceName))
	router.Use(otelchi.Middleware(app.Config.ServiceName, otelchi.WithChiRoutes(router)))
	router.Use(app.LogRequest)
	router.Use(app.SecureHeaders)

	router.Get("/healthcheck", app.healthCheckHandler)

	router.Route("/items", func(r chi.Router) {
		r.Use(app.Authenticate(app.UsersRepository, assets.EmbeddedFiles))

		r.With(app.RequirePermission(app.UsersRepository, "catalog:read")).Get("/", app.getItemsHandler)
		r.With(app.RequirePermission(app.UsersRepository, "catalog:read")).Get("/{id}", app.getItemHandler)
		r.With(app.RequirePermission(app.UsersRepository, "catalog:write")).Post("/", app.createItemHandler)
		r.With(app.RequirePermission(app.UsersRepository, "catalog:write")).Put("/{id}", app.updateItemHandler)
		r.With(app.RequirePermission(app.UsersRepository, "catalog:write")).Delete("/{id}", app.deleteItemHandler)
	})

	router.Get("/metrics", promhttp.Handler().ServeHTTP)

	return router
}
