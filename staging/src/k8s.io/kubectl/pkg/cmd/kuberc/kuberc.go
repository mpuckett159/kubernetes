/*
Copyright 2014 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package kuberc

import (
	"fmt"
	"github.com/spf13/cobra"
	"k8s.io/klog/v2"
	"os"
	"reflect"
	"strings"

	"sigs.k8s.io/yaml"
)

// Example kuberc.yaml file
// ---
//apiVersion: v1alpha1
//kind: Preferences
//command:
//  aliases:
//    getdbprod:
//      command: get pods
//      flags:
//        - -l what=database
//        - --namespace us-2-production
//    getdbdev:
//      command: get pods
//      flags:
//        - -l what=database
//        - --namespace us-2-development
//  overrides:
//    apply:
//      flags:
//        - --server-side=true
//    config:
//      flags:
//        - --kubeconfig "~/.kube/config"
//    config set:
//      flags:
//        - --set-raw-bytes
//    config view:
//      flags:
//        - --raw
//    delete:
//      flags:
//        - --confirm=true
//    default:
//      flags:
//        - --exec-auth-allowlist "/var/kubectl/exec/..."

const DefaultKubercPath = ".kube/kuberc"

type Preferences struct {
	Kind       string  `json:"kind,omitempty"`
	APIVersion string  `json:"apiVersion,omitempty"`
	Command    Command `json:"command,omitempty"`
}

type Command struct {
	Aliases   map[string]Alias    `json:"aliases,omitempty"`
	Overrides map[string]Override `json:"overrides,omitempty"`
}

type Alias struct {
	Command string   `json:"command,omitempty"`
	Flags   []string `json:"flags,omitempty"`
}

type Override struct {
	Flags []string `json:"flags,omitempty"`
}

// Handler is responsible for injecting aliases for commands and
// setting default flags arguments based on user's kuberc configuration.
type Handler interface {
	InjectAliases(rootCmd *cobra.Command, args []string)
	InjectOverrides(rootCmd *cobra.Command)
}

// DefaultKubercHandler implements AliasHandler
type DefaultKubercHandler struct {
	command Command
}

// NewDefaultKubercHandler instantiates the DefaultKubercHandler by reading the
// kuberc file.
func NewDefaultKubercHandler(kubercPath string) *DefaultKubercHandler {
	kuberc, err := LoadKubeRC(kubercPath)
	if err != nil {
		fmt.Println("error reading kuberc file")
	}
	return &DefaultKubercHandler{
		kuberc.Command,
	}
}

func (h *DefaultKubercHandler) InjectAliases(rootCmd *cobra.Command, args []string) {
	// Get the current command being passed
	var cmdArgs []string // all "non-flag" arguments
	for _, arg := range args {
		if strings.HasPrefix(arg, "-") {
			break
		}
		cmdArgs = append(cmdArgs, arg)
	}

	// Register all aliases
	for alias, command := range h.command.Aliases {
		commands := strings.Split(command.Command, " ")
		cmd, _, err := rootCmd.Find(commands)
		if err != nil {
			klog.Warningf("Command %q not found to set alias %q", commands, alias)
			continue
		}

		// do not allow shadowing built-ins
		if _, _, err := rootCmd.Find([]string{alias}); err == nil {
			klog.Warningf("Setting alias %q to a built-in command is not supported", alias)
			continue
		}

		// register alias
		cmd.Aliases = append(cmd.Aliases, alias)

		// inject alias flags if this is the command that is being targetted
		// fullAliasCmdPath is the command path defined in kuberc minus the
		// last command (which is being aliased) and the last arg of the cmdArgs
		// (which is what the alias would be). The cmdArgs will also include
		// kubectl so we will ignore the first entry in that array
		//
		// Example: kuberc defines alias "raw" for "config view" subcommand,
		// the user thus will supply "kubectl config raw" on the command line
		// so we take the command defined in the kuberc file and drop the last
		// subcommand, which is view, and replace it with the alias defined in
		// the kuberc file, giving us "kubectl config raw", then we check to
		// see if that is equal to commands supplied by the user.
		fullAliasCmdPath := append(commands[:len(commands)-1], alias)
		if reflect.DeepEqual(fullAliasCmdPath, cmdArgs[1:]) {
			klog.Infof("using alias %q, adding flags...", alias)
			cmd.Flags().Parse(command.Flags)
		}
	}
}

func (h *DefaultKubercHandler) InjectOverrides(rootCmd *cobra.Command) {
	// TODO: special case for "default" key that will set default flags for all commands
	for command, override := range h.command.Overrides {
		commands := strings.Split(command, " ")
		cmd, _, err := rootCmd.Find(commands)
		if err != nil {
			klog.Warningf("Command %q not found to set default flags for", command)
			continue
		}
		// inject default flags
		klog.Infof("adding flags %v for command %q", override.Flags, command)
		cmd.Flags().Parse(override.Flags)
	}
}

// LoadKubeRC reads kuberc file and stores the values in a Preferences struct
// that is then returned
func LoadKubeRC(path string) (Preferences, error) {
	kubercBytes, err := os.ReadFile(path)
	if err != nil {
		return Preferences{}, err
	}

	var preferences Preferences
	// TODO: (mpuckett159) This probably should be UnmarshalStrict but I'm not sure
	if err := yaml.Unmarshal(kubercBytes, &preferences); err != nil {
		fmt.Println("error unmarshalling the yaml")
	}
	klog.Infof("kuberc:\n%v", preferences)

	return preferences, nil
}
