package deployer

import (
	"bufio"
	"bytes"
	"encoding/json"
	goflag "flag"
	"fmt"
	"maps"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"text/template"

	"github.com/octago/sflags/gen/gpflag"
	"github.com/spf13/pflag"

	"sigs.k8s.io/kubetest2/pkg/artifacts"
	"sigs.k8s.io/kubetest2/pkg/types"

	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog/v2"

	"sigs.k8s.io/provider-ibmcloud-test-infra/kubetest2-tf/deployer/options"
	"sigs.k8s.io/provider-ibmcloud-test-infra/kubetest2-tf/pkg/ansible"
	"sigs.k8s.io/provider-ibmcloud-test-infra/kubetest2-tf/pkg/build"
	"sigs.k8s.io/provider-ibmcloud-test-infra/kubetest2-tf/pkg/providers"
	"sigs.k8s.io/provider-ibmcloud-test-infra/kubetest2-tf/pkg/providers/common"
	"sigs.k8s.io/provider-ibmcloud-test-infra/kubetest2-tf/pkg/providers/powervs"
	"sigs.k8s.io/provider-ibmcloud-test-infra/kubetest2-tf/pkg/providers/vpc"
	"sigs.k8s.io/provider-ibmcloud-test-infra/kubetest2-tf/pkg/terraform"

	"sigs.k8s.io/kubetest2/pkg/metadata"
)

const (
	Name              = "tf"
	inventoryTemplate = `[masters]
{{range .Masters}}{{.}}
{{end}}
[workers]
{{range .Workers}}{{.}}
{{end}}
`
)

var GitTag string

type AnsibleInventory struct {
	Masters []string
	Workers []string
}

// Add additional Linux package dependencies here, used by checkDependencies()
var dependencies = []string{"terraform", "ansible", "kubectl"}

func (i *AnsibleInventory) addMachine(mtype string, value string) {
	v := reflect.ValueOf(i).Elem().FieldByName(mtype)
	if v.IsValid() {
		v.Set(reflect.Append(v, reflect.ValueOf(value)))
	}
}

type deployer struct {
	BuildOptions *options.BuildOptions

	commonOptions types.Options
	doInit        sync.Once
	logsDir       string
	provider      providers.Provider
	tmpDir        string
	machineIPs    []string

	RepoRoot              string            `desc:"The path to the root of the local kubernetes repo. Necessary to call certain scripts. Defaults to the current directory. If operating in legacy mode, this should be set to the local kubernetes/kubernetes repo."`
	IgnoreClusterDir      bool              `desc:"Ignore the cluster folder if exists"`
	RetryOnTfFailure      int               `desc:"Retry on Terraform Apply Failure"`
	BreakKubetestOnUpfail bool              `desc:"Breaks kubetest2 when up fails"`
	Playbook              string            `desc:"Name of ansible playbook to be run"`
	ExtraVars             map[string]string `desc:"Passes extra-vars to ansible playbook, enter a string of key=value pairs"`
	SetKubeconfig         bool              `desc:"Flag to set kubeconfig"`
	TargetProvider        string            `desc:"provider value to be used(powervs, vpc)"`
}

func (d *deployer) Version() string {
	return GitTag
}

func (d *deployer) init() error {
	var err error
	d.doInit.Do(func() { err = d.initialize() })
	return err
}

func (d *deployer) initialize() error {
	klog.Info("Check if package dependencies are installed in the environment")
	if d.commonOptions.ShouldBuild() {
		if err := d.verifyBuildFlags(); err != nil {
			return fmt.Errorf("init failed to check build flags: %s", err)
		}
	}
	if err := d.checkDependencies(); err != nil {
		return err
	}

	if d.TargetProvider == "vpc" {
		d.provider = vpc.VPCProvider
	} else {
		d.provider = powervs.PowerVSProvider
	}

	common.CommonProvider.Initialize()
	d.tmpDir = common.CommonProvider.ClusterName
	if _, err := os.Stat(d.tmpDir); os.IsNotExist(err) {
		err := os.Mkdir(d.tmpDir, 0755)
		if err != nil {
			return fmt.Errorf("failed to create dir: %s", d.tmpDir)
		}
	} else if !d.IgnoreClusterDir {
		return fmt.Errorf("directory named %s already exist, please choose a different cluster-name", d.tmpDir)
	}
	return nil
}

var _ types.Deployer = &deployer{}

func New(opts types.Options) (types.Deployer, *pflag.FlagSet) {
	d := &deployer{
		commonOptions: opts,
		logsDir:       filepath.Join(artifacts.BaseDir(), "logs"),
		BuildOptions: &options.BuildOptions{
			CommonBuildOptions: &build.Options{
				Builder:         &build.NoopBuilder{},
				Stager:          &build.NoopStager{},
				Strategy:        "make",
				TargetBuildArch: "linux/ppc64le",
				COSCredType:     "shared",
			},
		},
		RetryOnTfFailure: 1,
		Playbook:         "install-k8s.yml",
		SetKubeconfig:    true,
		TargetProvider:   "powervs",
	}
	flagSet, err := gpflag.Parse(d)
	if err != nil {
		klog.Fatalf("couldn't parse flagset for deployer struct: %s", err)
	}
	klog.InitFlags(nil)
	flagSet.AddGoFlagSet(goflag.CommandLine)
	fs := bindFlags()
	flagSet.AddFlagSet(fs)
	return d, flagSet
}

func bindFlags() *pflag.FlagSet {
	flags := pflag.NewFlagSet(Name, pflag.ContinueOnError)
	common.CommonProvider.BindFlags(flags)
	vpc.VPCProvider.BindFlags(flags)
	powervs.PowerVSProvider.BindFlags(flags)
	return flags
}

func (d *deployer) Up() error {
	if err := d.init(); err != nil {
		return fmt.Errorf("up failed to init: %s", err)
	}

	err := common.CommonProvider.DumpConfig(d.tmpDir)
	if err != nil {
		return fmt.Errorf("failed to dump common flags: %s", d.tmpDir)
	}

	err = d.provider.DumpConfig(d.tmpDir)
	if err != nil {
		return fmt.Errorf("failed to dumpconfig to: %s and err: %+v", d.tmpDir, err)
	}
	for i := 0; i <= d.RetryOnTfFailure; i++ {
		path, err := terraform.Apply(d.tmpDir, d.TargetProvider)
		op, oerr := terraform.Output(d.tmpDir, d.TargetProvider)
		if err != nil {
			if i == d.RetryOnTfFailure {
				fmt.Printf("terraform.Output: %s\nterraform.Output error: %v\n", op, oerr)
				if !d.BreakKubetestOnUpfail {
					return fmt.Errorf("terraform Apply failed. Error: %v", err)
				}
				klog.Infof("Terraform Apply failed. Look into it and delete the resources")
				klog.Infof("terraform.Apply error: %v", err)
				os.Exit(1)
			}
			continue
		} else {
			fmt.Printf("terraform.Output: %s\nterraform.Output error: %v\n", op, oerr)
			fmt.Printf("Terraform State at: %s\n", path)
			break
		}
	}
	inventory := AnsibleInventory{}
	tfMetaOutput, err := terraform.Output(d.tmpDir, d.TargetProvider)
	if err != nil {
		return err
	}
	var tfOutput map[string][]interface{}
	data, err := json.Marshal(tfMetaOutput)
	if err != nil {
		return fmt.Errorf("error while marshaling data %v", err)
	}
	json.Unmarshal(data, &tfOutput)
	for _, machineType := range []string{"Masters", "Workers"} {
		if val, ok := tfOutput[strings.ToLower(machineType)]; !ok {
			fmt.Errorf("error while unmarshaling machine IPs from terraform output")
		} else {
			for _, machineIp := range val {
				inventory.addMachine(machineType, machineIp.(string))
				d.machineIPs = append(d.machineIPs, machineIp.(string))
			}
		}
	}
	klog.Infof("Kubernetes cluster node inventory: %+v", inventory)
	t := template.New("Ansible inventory file")

	t, err = t.Parse(inventoryTemplate)
	if err != nil {
		return fmt.Errorf("template parse failed: %v", err)
	}

	inventoryFile, err := os.Create(filepath.Join(d.tmpDir, "hosts"))
	if err != nil {
		klog.Errorf("Error while creating a file: %v", err)
		return fmt.Errorf("failed to create inventory file: %v", err)
	}

	if err = t.Execute(inventoryFile, inventory); err != nil {
		return fmt.Errorf("template execute failed: %v", err)
	}

	common.CommonProvider.ExtraCerts = strings.Join(inventory.Masters, ",")

	ansibleParams, err := json.Marshal(common.CommonProvider)
	if err != nil {
		return fmt.Errorf("failed to marshal provider into JSON: %v", err)
	}
	klog.Infof("Ansible params exposed under groupvars: %v", string(ansibleParams))
	// Unmarshalling commonJSON into map to add extra-vars
	combinedAnsibleVars := map[string]string{}
	if err = json.Unmarshal(ansibleParams, &combinedAnsibleVars); err != nil {
		return fmt.Errorf("failed to unmarshal ansible group variables: %v", err)
	}

	// Add-in the extra-vars set to the final set.
	maps.Insert(combinedAnsibleVars, maps.All(d.ExtraVars))
	klog.Infof("Updated ansible variables with extra vars: %+v", combinedAnsibleVars)
	err = ansible.Playbook(d.tmpDir, filepath.Join(d.tmpDir, "hosts"), d.Playbook, combinedAnsibleVars)
	if err != nil {
		return fmt.Errorf("failed to run ansible playbook: %v", err)
	}

	if d.SetKubeconfig {
		if err = setKubeconfig(inventory.Masters[0]); err != nil {
			return fmt.Errorf("failed to setKubeconfig: %v", err)
		}
		fmt.Printf("KUBECONFIG set to: %s\n", os.Getenv("KUBECONFIG"))
	}

	if isUp, err := d.IsUp(); err != nil {
		klog.Warningf("failed to check if cluster is up: %v", err)
	} else if isUp {
		klog.V(1).Info("cluster reported as up")
	} else {
		klog.Error("cluster reported as down")
	}

	klog.Infof("Dumping cluster info..")
	if err := d.DumpClusterLogs(); err != nil {
		klog.Warningf("Dumping cluster logs at the end of Up() failed: %v", err)
	}
	return nil
}

// setKubeconfig overrides the server IP addresses in the kubeconfig and set the KUBECONFIG environment
func setKubeconfig(host string) error {
	_, err := os.Stat(common.CommonProvider.KubeconfigPath)
	if err != nil {
		return fmt.Errorf("failed to locate the kubeconfig file: %v", err)
	}

	config, err := clientcmd.LoadFromFile(common.CommonProvider.KubeconfigPath)
	if err != nil {
		klog.Errorf("failed to load the kubeconfig file. error: %v", err)
	}
	for i := range config.Clusters {
		surl, err := url.Parse(config.Clusters[i].Server)
		if err != nil {
			return fmt.Errorf("failed while Parsing the URL: %s", config.Clusters[i].Server)
		}
		_, port, err := net.SplitHostPort(surl.Host)
		if err != nil {
			return fmt.Errorf("errored while SplitHostPort")
		}
		surl.Host = net.JoinHostPort(host, port)
		config.Clusters[i].Server = surl.String()
	}
	clientcmd.WriteToFile(*config, common.CommonProvider.KubeconfigPath)
	kubecfgAbsPath, err := filepath.Abs(common.CommonProvider.KubeconfigPath)
	if err != nil {
		return fmt.Errorf("failed to create absolute path for the kubeconfig file: %v", err)
	}
	if err = os.Setenv("KUBECONFIG", kubecfgAbsPath); err != nil {
		return fmt.Errorf("failed to set the KUBECONFIG environment variable")
	}
	return nil
}

func (d *deployer) Down() error {

	if err := d.init(); err != nil {
		return fmt.Errorf("down failed to init: %s", err)
	}
	err := terraform.Destroy(d.tmpDir, d.TargetProvider)
	if err != nil {
		if common.CommonProvider.IgnoreDestroy {
			klog.Infof("terraform.Destroy failed: %v", err)
		} else {
			return fmt.Errorf("terraform.Destroy failed: %v", err)
		}
	}
	return nil
}

func (d *deployer) IsUp() (up bool, err error) {
	var lines []string
	command := []string{
		"kubectl",
		"get", "nodes",
		"-o=name",
	}
	klog.Infof("About to run: %s", command)
	cmd := exec.Command(command[0], command[1:]...)
	var buff bytes.Buffer
	cmd.Stdout = &buff
	if err = cmd.Run(); err != nil {
		return false, metadata.NewJUnitError(err, strings.Join(lines, "\n"))
	}
	scanner := bufio.NewScanner(&buff)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	if len(lines) == 0 {
		return false, fmt.Errorf("project had no nodes active: %s", common.CommonProvider.ClusterName)
	}
	return true, nil
}

// checkDependencies determines if the required packages are installed before
// the test execution begins, providing a fail-fast route for exit if the packages are not found.
func (d *deployer) checkDependencies() error {
	for _, dependency := range dependencies {
		if _, err := exec.LookPath(dependency); err != nil {
			return fmt.Errorf("failed to find %s in the test environment: %s", dependency, err)
		}
	}
	return nil
}
