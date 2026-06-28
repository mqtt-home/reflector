package reflector

// Prefix is the annotation namespace used by reflector. It is kept byte-for-byte
// identical to the original emberstack/kubernetes-reflector so existing source and
// mirror objects are recognised without any migration.
const Prefix = "reflector.v1.k8s.emberstack.com"

const (
	// Source annotations (set by the user on the source secret/configmap).
	AnnReflectionAllowed           = Prefix + "/reflection-allowed"
	AnnReflectionAllowedNamespaces = Prefix + "/reflection-allowed-namespaces"
	AnnReflectionAllowedNsSelector = Prefix + "/reflection-allowed-namespaces-selector"
	AnnReflectionAutoEnabled       = Prefix + "/reflection-auto-enabled"
	AnnReflectionAutoNamespaces    = Prefix + "/reflection-auto-namespaces"
	AnnReflectionAutoNsSelector    = Prefix + "/reflection-auto-namespaces-selector"

	// Reflects is set by the user on a mirror to point it at its source ("namespace/name"),
	// and is also written by reflector onto every mirror it creates.
	AnnReflects = Prefix + "/reflects"

	// Meta annotations written by reflector onto mirrors.
	AnnMetaAutoReflects     = Prefix + "/auto-reflects"
	AnnMetaReflectedVersion = Prefix + "/reflected-version"
	AnnMetaReflectedAt      = Prefix + "/reflected-at"
)
