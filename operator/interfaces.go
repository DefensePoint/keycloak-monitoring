// Package operator provides the operator metrics domain model and services.
package operator

import "gorm.io/gorm"

// DB is a local interface for database operations.
// This abstracts the database client so the operator domain can be tested
// without depending on the concrete database.Client implementation.
type DB interface {
	DB() *gorm.DB
}
