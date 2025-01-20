package runtime

type Env string

const (
	Development Env = "dev"
	Test        Env = "test"
	Production  Env = "prod"
)