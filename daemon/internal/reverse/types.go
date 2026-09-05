package reverse

import (
	"encoding/json"
	"time"
)

type KidID string

type KidName string

type Ticket string

type TicketHash string

type Role int

const (
	RoleNone Role = iota
	RoleParent
	RoleKid
)

type Endpoint string

type Phase int

const (
	PhaseSearch Phase = iota
	PhaseDial
	PhaseOffer
	PhaseServe
	PhaseBackoff
)

type Presence int

const (
	PresenceWaiting Presence = iota
	PresenceOffline
	PresenceLive
)

type Record struct {
	ID         KidID      `json:"id"`
	Name       KidName    `json:"name"`
	URL        string     `json:"url,omitempty"`
	Token      string     `json:"token,omitempty"`
	TicketHash TicketHash `json:"ticket_hash,omitempty"`
	PairedAt   time.Time  `json:"paired_at"`
}

type File struct {
	Kids []Record `json:"kids"`
}

type Member struct {
	ID     KidID           `json:"id"`
	Name   KidName         `json:"name"`
	Live   bool            `json:"live"`
	Status json.RawMessage `json:"status,omitempty"`
	Asks   json.RawMessage `json:"asks,omitempty"`
	Look   json.RawMessage `json:"look,omitempty"`
}

type Household struct {
	Kids []Member `json:"kids"`
}

type Offer struct {
	ID   KidID   `json:"id"`
	Name KidName `json:"name"`
}

type Resume struct {
	ID     KidID  `json:"id"`
	Ticket Ticket `json:"ticket"`
}

type Accept struct {
	Ticket Ticket `json:"ticket"`
}

type ResumeOK struct{}

type Reject struct {
	Reason RejectReason `json:"reason"`
}

type RejectReason int

const (
	RejectBadTicket RejectReason = iota
	RejectForeignParent
	RejectNotPrivate
	RejectParentRole
)

type Op struct {
	Corr    uint64          `json:"corr"`
	Method  string          `json:"method"`
	Path    string          `json:"path"`
	Body    json.RawMessage `json:"body,omitempty"`
	IdemKey string          `json:"idem_key,omitempty"`
}

type OpResult struct {
	Corr   uint64          `json:"corr"`
	Status int             `json:"status"`
	Body   json.RawMessage `json:"body,omitempty"`
}

type Kind int

const (
	KindOffer Kind = iota
	KindResume
	KindAccept
	KindResumeOK
	KindReject
	KindOp
	KindResult
)

type Frame struct {
	Kind     Kind
	Offer    *Offer
	Resume   *Resume
	Accept   *Accept
	ResumeOK *ResumeOK
	Reject   *Reject
	Op       *Op
	Result   *OpResult
}

type SessionFile struct {
	Parent Endpoint `json:"parent"`
	Ticket Ticket   `json:"ticket"`
	ID     KidID    `json:"id"`
}
