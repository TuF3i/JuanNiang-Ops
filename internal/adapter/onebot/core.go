package onebot

type Config struct {
	Addr   string
	Port   int
	Token  string
	Admins []string
	Enable bool
}

type Adapter struct {
	
}