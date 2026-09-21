package collection

type KV struct{ K, V string }

type Part struct {
	Name        string
	IsFile      bool
	Value       string
	ContentType string
}

type Body struct {
	Kind  string
	Raw   string
	Parts []Part
	File  string
}

type Auth struct {
	Type                  string
	Token                 string
	Username, Password    string
	Key, Value, Placement string
}

type Request struct {
	Path       string
	Name       string
	Seq        int
	Method     string
	URL        string
	Query      []KV
	PathParams []KV
	Headers    []KV
	Body       Body
	Auth       *Auth
	TimeoutMS  int
	Vars       []KV
}

type Defaults struct {
	Headers []KV
	Vars    []KV
	Auth    *Auth
}
