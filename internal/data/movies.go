package data

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/lib/pq"
	"greenlight.alexbates.net/internal/validator"
)

type Movie struct {
	ID        int64     `json:"id"`                       // Unique integer ID for the movie
	CreatedAt time.Time `json:"created_at"`               // Timestamp for when the movie is added to the database
	Title     string    `json:"title"`                    // Movie title
	Year      int32     `json:"year,omitempty"`           // Movie release year
	Runtime   Runtime   `json:"runtime,omitempty,string"` // Movie runtime (in minutes)
	Genres    []string  `json:"genres,omitempty"`         // Slice of genres for the movie
	Version   int32     `json:"version"`                  // The version number starts at 1 and will be incremented each time the movie information is updated
}

type MovieModel struct {
	DB *sql.DB
}

// Insert accepts a point to a movie struct which contains the data for the new record.
// Because we take a pointer to Movie, when we call 'Scan()' we're updating the values at the location the parameter points to
func (m MovieModel) Insert(movie *Movie) error {

	// Define SQL query
	query := `
		INSERT INTO movies (title, year, runtime, genres)
		VALUES ($1, $2, $3, $4)	
		RETURNING id, created_at, version
	`

	// Create an args slice containing the values for the placeholder parameters from the movie struct
	// This could be done below but it's more readable to do it here
	args := []any{movie.Title, movie.Year, movie.Runtime, pq.Array(movie.Genres)}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	// Execute the query using the DB's QueryRow() method and scan the result into the movie struct
	return m.DB.QueryRowContext(ctx, query, args...).Scan(&movie.ID, &movie.CreatedAt, &movie.Version)
}

func (m MovieModel) Get(id int64) (*Movie, error) {
	if id < 1 {
		return nil, ErrRecordNotFound
	}

	query := `
		SELECT id, created_at, title, year, runtime, genres, version
		FROM movies
		WHERE id = $1
	`

	// Declare a movie struct to hold the data we get back from the database
	var movie Movie

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)

	defer cancel()

	err := m.DB.QueryRowContext(ctx, query, id).Scan(
		&movie.ID,
		&movie.CreatedAt,
		&movie.Title,
		&movie.Year,
		&movie.Runtime,
		pq.Array(&movie.Genres),
		&movie.Version,
	)

	if err != nil {
		switch {
		case errors.Is(err, sql.ErrNoRows):
			return nil, ErrRecordNotFound
		default:
			return nil, err
		}
	}

	return &movie, nil
}

func (m MovieModel) Update(movie *Movie) error {
	query := `
		UPDATE movies
		SET title = $1, year = $2, runtime = $3, genres = $4, version = version + 1
		WHERE id = $5 AND version = $6
		RETURNING version
	`

	args := []interface{}{
		movie.Title,
		movie.Year,
		movie.Runtime,
		pq.Array(movie.Genres),
		movie.ID,
		movie.Version,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	err := m.DB.QueryRowContext(ctx, query, args...).Scan(&movie.Version)
	if err != nil {
		switch {
		case errors.Is(err, sql.ErrNoRows):
			return ErrEditConflict
		default:
			return err
		}
	}

	return nil
}

func (m MovieModel) Delete(id int64) error {
	if id < 1 {
		return ErrRecordNotFound
	}

	query := `
		DELETE FROM movies
		WHERE id = $1
	`

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	result, err := m.DB.ExecContext(ctx, query, id)
	if err != nil {
		return err
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if rowsAffected == 0 {
		return ErrRecordNotFound
	}

	return nil
}

func (m MovieModel) GetAll(title string, genres []string, filters Filters) ([]*Movie, Metadata, error) {
	query := fmt.Sprintf(`
		SELECT count(*) OVER(), id, created_at, title, year, runtime, genres, version
		FROM movies
		WHERE (to_tsvector('simple', title) @@ plainto_tsquery('simple', $1) OR $1 = '')
		AND (genres @> $2 OR $2 = '{}')
		ORDER BY %s %s, id ASC
		LIMIT $3 OFFSET $4
	`, filters.sortColumn(), filters.sortDirection())

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	args := []any{title, pq.Array(genres), filters.limit(), filters.offset()}
	rows, err := m.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, Metadata{}, err
	}

	defer rows.Close()

	totalRecords := 0
	movies := []*Movie{}

	for rows.Next() {
		var movie Movie

		err := rows.Scan(
			&totalRecords,
			&movie.ID,
			&movie.CreatedAt,
			&movie.Title,
			&movie.Year,
			&movie.Runtime,
			pq.Array(&movie.Genres),
			&movie.Version,
		)
		if err != nil {
			return nil, Metadata{}, err
		}

		movies = append(movies, &movie)
	}

	if err = rows.Err(); err != nil {
		return nil, Metadata{}, err
	}

	metadata := calculateMetadata(totalRecords, filters.Page, filters.PageSize)

	return movies, metadata, nil
}

func ValidateMovie(v *validator.Validator, movie *Movie) {
	v.Check(movie.Title != "", "title", "must be provided")
	v.Check(len(movie.Title) <= 500, "title", "must not be more than 500 bytes long")

	v.Check(movie.Year != 0, "year", "must be provided")
	v.Check(movie.Year >= 1888, "year", "must be greater than 1888")
	v.Check(movie.Year <= int32(time.Now().Year()), "year", "must not be in the future")

	v.Check(movie.Runtime != 0, "runtime", "must be provided")
	v.Check(movie.Runtime > 0, "runtime", "must be a positive integer")

	v.Check(movie.Genres != nil, "genres", "must be provided")
	v.Check(len(movie.Genres) >= 1, "genres", "must contain at least 1 genre")
	v.Check(len(movie.Genres) <= 5, "genres", "must not contain more than 5 genres")
	v.Check(validator.Unique(movie.Genres), "genres", "must not contain duplicate values")
}
