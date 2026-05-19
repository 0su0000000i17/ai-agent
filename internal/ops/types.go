package ops

type DecisionType string

const (
	DecisionChatReply DecisionType = "chat_reply"
	DecisionOperation DecisionType = "operation"
)

type OperationKind string

const (
	OperationInsertText  OperationKind = "insert_text"
	OperationReplaceText OperationKind = "replace_text"
	OperationNewLine     OperationKind = "new_line"
	OperationSelectAll   OperationKind = "select_all"
	OperationSave        OperationKind = "save"
	OperationCopy        OperationKind = "copy"
	OperationPaste       OperationKind = "paste"
	OperationScroll      OperationKind = "scroll"
	OperationOpenApp     OperationKind = "open_app"
)

type Decision struct {
	Type   DecisionType `json:"type"`
	Reply  string       `json:"reply,omitempty"`
	Reason string       `json:"reason,omitempty"`

	Operation Operation `json:"operation,omitempty"`
}

type Operation struct {
	Kind OperationKind `json:"kind"`

	TargetHint string `json:"target_hint,omitempty"`
	AppQuery   string `json:"app_query,omitempty"`

	Text      string `json:"text,omitempty"`
	Direction string `json:"direction,omitempty"`
}
