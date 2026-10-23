package onebot

type Config struct {
	Addr   string // 监听地址, 格式 host:port
	Port   int
	Token  string
	Admin  []string
	Enable bool
}

type Adapter struct {
	cfg Config
}
