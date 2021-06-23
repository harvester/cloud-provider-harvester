package ccm

// cloud-config.conf
// [cluster]
// name = <cluster name>
// [harvester]
// masterUrl = <Harvester cluster master url>
// token = <Harvester service account token>
// namespace = <namespace in Harvester where to create resources>
type cloudConfig struct {
	Cluster   clusterConfig
	Harvester harvesterConfig
}

type clusterConfig struct {
	Name string
}

type harvesterConfig struct {
	Server      string
	Token       string
	Certificate string `toml:"certificate-authority-data"`
	Namespace   string
}
