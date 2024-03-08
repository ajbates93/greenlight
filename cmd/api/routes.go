package main

import (
	"net/http"

	"github.com/julienschmidt/httprouter"
)

func (app *application) routes() http.Handler {
	// Initialise a new httprouter router instance.
	router := httprouter.New()

	// Define custom error handlers for 404 and 405 responses.
	router.NotFound = http.HandlerFunc(app.notFoundResponse)
	router.MethodNotAllowed = http.HandlerFunc(app.methodNotAllowedResponse)
	router.HandlerFunc(http.MethodGet, "/v1/healthcheck", app.healthcheckHandler)

	// Set up route handlers
	router.HandlerFunc(http.MethodGet, "/v1/movies", app.listMoviesHandler)
	router.HandlerFunc(http.MethodPost, "/v1/movies", app.createMovieHandler)
	router.HandlerFunc(http.MethodGet, "/v1/movies/:id", app.showMovieHandler)
	router.HandlerFunc(http.MethodPatch, "/v1/movies/:id", app.updateMovieHandler)
	router.HandlerFunc(http.MethodDelete, "/v1/movies/:id", app.deleteMovieHandler)

	// Return the httprouter instance (wrapped in our panic recovery middleware).
	return app.recoverPanic(app.rateLimit(router))
}
