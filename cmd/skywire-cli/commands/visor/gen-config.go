package visor

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/sirupsen/logrus"
	"github.com/skycoin/dmsg/cipher"
	coinCipher "github.com/skycoin/skycoin/src/cipher"
	"github.com/skycoin/skycoin/src/util/logging"
	"github.com/spf13/cobra"

	"github.com/skycoin/skywire/pkg/visor/visorconfig"
)

func init() {
	RootCmd.AddCommand(genConfigCmd)
}

var (
	sk            cipher.SecKey
	output        string
	replace       bool
	testEnv       bool
	packageConfig bool
	hypervisor    bool
	hypervisorPKs string
)

func init() {
	genConfigCmd.Flags().Var(&sk, "sk", "if unspecified, a random key pair will be generated.")
	genConfigCmd.Flags().StringVarP(&output, "output", "o", "skywire-config.json", "path of output config file.")
	genConfigCmd.Flags().BoolVarP(&replace, "replace", "r", false, "whether to allow rewrite of a file that already exists (this retains the keys).")
	genConfigCmd.Flags().BoolVarP(&packageConfig, "package", "p", false, "use defaults for package-based installations")
	genConfigCmd.Flags().BoolVarP(&testEnv, "testenv", "t", false, "whether to use production or test deployment service.")
	genConfigCmd.Flags().BoolVar(&hypervisor, "is-hypervisor", false, "whether to generate config to run this visor as a hypervisor.")
	genConfigCmd.Flags().StringVar(&hypervisorPKs, "hypervisor-pks", "", "public keys of hypervisors that should be added to this visor")
}

var genConfigCmd = &cobra.Command{
	Use:   "gen-config",
	Short: "Generates a config file",
	PreRun: func(_ *cobra.Command, _ []string) {
		var err error
		if output, err = filepath.Abs(output); err != nil {
			logger.WithError(err).Fatal("Invalid output provided.")
		}
	},
	Run: func(_ *cobra.Command, _ []string) {
		mLog := logging.NewMasterLogger()
		mLog.SetLevel(logrus.InfoLevel)

		// Read in old config (if any) and obtain old secret key.
		// Otherwise, we generate a new random secret key.
		var sk cipher.SecKey
		if oldConf, ok := readOldConfig(mLog, output, replace); !ok {
			_, sk = cipher.GenerateKeyPair()
		} else {
			sk = oldConf.SK
		}

		// Determine config type to generate.
		var genConf func(log *logging.MasterLogger, confPath string, sk *cipher.SecKey, hypervisor bool) (*visorconfig.V1, error)

		// to be improved later
		if packageConfig {
			genConf = visorconfig.MakePackageConfig
		} else if testEnv {
			genConf = visorconfig.MakeTestConfig
		} else {
			genConf = visorconfig.MakeDefaultConfig
		}

		// Generate config.
		conf, err := genConf(mLog, output, &sk, hypervisor)
		if err != nil {
			logger.WithError(err).Fatal("Failed to create config.")
		}

		if hypervisorPKs != "" {
			keys := strings.Split(hypervisorPKs, ",")
			for _, key := range keys {
				keyParsed, err := coinCipher.PubKeyFromHex(strings.TrimSpace(key))
				if err != nil {
					logger.WithError(err).Fatalf("Failed to parse hypervisor private key: %s.", key)
				}
				conf.Hypervisors = append(conf.Hypervisors, cipher.PubKey(keyParsed))
			}

		}

		// Save config to file.
		if err := conf.Flush(); err != nil {
			logger.WithError(err).Fatal("Failed to flush config to file.")
		}

		// Print results.
		j, err := json.MarshalIndent(conf, "", "\t")
		if err != nil {
			logger.WithError(err).Fatal("An unexpected error occurred. Please contact a developer.")
		}
		logger.Infof("Updated file '%s' to: %s", output, j)
	},
}

func readOldConfig(log *logging.MasterLogger, confPath string, replace bool) (*visorconfig.V1, bool) {
	raw, err := ioutil.ReadFile(confPath) //nolint:gosec
	if err != nil {
		if os.IsNotExist(err) {
			return nil, false
		}
		logger.WithError(err).Fatal("Unexpected error occurred when attempting to read old config.")
	}

	if !replace {
		logger.Fatal("Config file already exists. Specify the 'replace,r' flag to replace this.")
	}

	conf, err := visorconfig.Parse(log, confPath, raw)
	if err != nil {
		logger.WithError(err).Fatal("Failed to parse old config file.")
	}

	return conf, true
}
