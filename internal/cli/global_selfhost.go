package cli

// earlySelfHostScan / earlyLanRelaxScan are set from a full argv scan at Main() startup
// (before subcommand routing) so flags before `run` / `generate` are honored.
var earlySelfHostScan, earlyLanRelaxScan bool

// wizardSelfHostEffective / wizardLanRelaxEffective are the combined early-scan + root
// flag.Parse() booleans for the interactive wizard (set in Main before runWizard).
var wizardSelfHostEffective, wizardLanRelaxEffective bool

func scanEarlySelfHostFlags(argv []string) {
	earlySelfHostScan = false
	earlyLanRelaxScan = false
	for _, a := range argv {
		switch a {
		case "--self-host":
			earlySelfHostScan = true
		case "--lan-relax":
			earlyLanRelaxScan = true
		}
	}
}

func sliceContainsExact(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}

// injectGlobalSelfHostPrepends prepends --self-host / --lan-relax when they appeared
// earlier in os.Args but were not forwarded into the run/generate slice.
func injectGlobalSelfHostPrepends(ra []string) []string {
	var pre []string
	if earlySelfHostScan && !sliceContainsExact(ra, "--self-host") {
		pre = append(pre, "--self-host")
	}
	if earlyLanRelaxScan && !sliceContainsExact(ra, "--lan-relax") {
		pre = append(pre, "--lan-relax")
	}
	if len(pre) == 0 {
		return ra
	}
	return append(pre, ra...)
}
