package look

type HostKind int

const (
	HostDown HostKind = iota
	HostBedtime
	HostLocked
	HostOn
	HostIdle
)

type HostFace struct {
	Kind HostKind
	App  string
}

func HostFaceFrom(reachable, bedtimeActive, locked bool, focusedApp string) HostFace {
	if !reachable {
		return HostFace{Kind: HostDown}
	}
	if bedtimeActive {
		return HostFace{Kind: HostBedtime}
	}
	if locked {
		return HostFace{Kind: HostLocked}
	}
	if focusedApp != "" {
		return HostFace{Kind: HostOn, App: focusedApp}
	}
	return HostFace{Kind: HostIdle}
}

func (f HostFace) LED() bool {
	return f.Kind == HostOn
}

func (f HostFace) Caption() string {
	switch f.Kind {
	case HostDown:
		return "down"
	case HostBedtime:
		return "bedtime"
	case HostLocked:
		return "locked"
	case HostOn:
		return "on " + f.App
	default:
		return ""
	}
}
