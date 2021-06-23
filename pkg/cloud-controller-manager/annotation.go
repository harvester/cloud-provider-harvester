package ccm

const (
	// Annotation key
	prefix                      = "loadbalancer.harvesterhci.io/"
	loadBalancerDescription     = prefix + "description"
	loadBalancerType            = prefix + "type"
	healthCheckPort             = prefix + "healthCheckPort"
	healthCheckSuccessThreshold = prefix + "healthCheckSuccessThreshold"
	healthCheckFailureThreshold = prefix + "healthCheckFailureThreshold"
	healthCheckPeriodSeconds    = prefix + "healthCheckPeriodSeconds"
	healthCheckTimeoutSeconds   = prefix + "healthCheckTimeoutSeconds"

	// Default value
	defaultSuccessThreshold = 1
	defaultFailThreshold    = 3
	defaultPeriodSeconds    = 5
	defaultTimeoutSeconds   = 3
)
