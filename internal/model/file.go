package model

type File struct {
	URL         string `json:"url,omitempty"`
	ContentType string `json:"content_type,omitempty"`
	RealType    string `json:"real_type,omitempty"`
	OrigName    string `json:"orig_name,omitempty"`
	Name        string `json:"name,omitempty"`
	Size        int64  `json:"size,omitempty"`
	Status      int    `json:"status,omitempty"`
	ErrorMsg    string `json:"error_msg,omitempty"`
}
