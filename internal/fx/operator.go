package fx

import (
	"go.uber.org/fx"

	"repos.defensepoint.com/iam/oss-pmt/kmt/operator"
	operatorpostgres "repos.defensepoint.com/iam/oss-pmt/kmt/operator/postgres"
	"repos.defensepoint.com/iam/oss-pmt/kmt/pkg/database"
)

// OperatorModule provides operator domain dependencies.
var OperatorModule = fx.Module("operator",
	fx.Provide(
		provideOperatorRepository,
		provideOperatorService,
	),
)

// provideOperatorRepository creates an operator repository.
func provideOperatorRepository(db *database.Client) operator.Repository {
	return operatorpostgres.NewRepository(db)
}

// provideOperatorService creates an operator service.
func provideOperatorService(repo operator.Repository) operator.Service {
	return operator.NewService(repo)
}
