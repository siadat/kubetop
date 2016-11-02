package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/fatih/color"
	"github.com/olekukonko/tablewriter"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/pkg/api/v1"
	"k8s.io/client-go/tools/clientcmd"
)

var (
	colorNode       = color.New(color.FgYellow).SprintFunc()
	colorPod        = color.New(color.FgCyan).SprintFunc()
	colorService    = color.New(color.FgBlue).SprintFunc()
	colorDeployment = color.New(color.FgMagenta).SprintFunc()
	colorFailed     = color.New(color.FgRed).SprintFunc()
)

type (
	Row  []string
	Rows []Row
)

func (r Rows) Len() int      { return len(r) }
func (r Rows) Swap(i, j int) { r[i], r[j] = r[j], r[i] }
func (r Rows) Less(i, j int) bool {
	return fmt.Sprintf("%s", r[i]) < fmt.Sprintf("%s", r[j])
}

func main() {
	log.SetFlags(log.Lshortfile)
	usr, err := user.Current()
	if err != nil {
		log.Fatal(err)
	}
	configFilepath := filepath.Join(usr.HomeDir, ".kube", "config")

	fmt.Printf("Using %s\n", configFilepath)
	config, err := clientcmd.BuildConfigFromFlags("", configFilepath)
	if err != nil {
		log.Fatal(err)
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		log.Fatal(err)
	}

	var ch = make(chan Rows)
	var rows Rows
	for {
		rows = make(Rows, 0)
		go func() {
			for r := range ch {
				rows = append(rows, r...)
			}
		}()

		var wg sync.WaitGroup
		wg.Add(4)
		go func() { defer wg.Done(); getNodes(ch, clientset) }()
		go func() { defer wg.Done(); getServices(ch, clientset) }()
		go func() { defer wg.Done(); getDeployments(ch, clientset) }()
		go func() { defer wg.Done(); getPods(ch, clientset) }()
		wg.Wait()

		clear()
		sort.Sort(rows)
		render(Row{
			"Type",
			"Namespace",
			"Name",
			"Status",
			"IPs",
			"Age",
		}, rows)
		time.Sleep(500 * time.Millisecond)
	}
}

func getNodes(ch chan Rows, clientset *kubernetes.Clientset) {
	nodes, err := clientset.Core().Nodes().List(v1.ListOptions{})
	if err != nil {
		log.Fatal(err)
	}

	var rows Rows
	for _, node := range nodes.Items {
		var statuses []string
		if len(node.Status.Phase) > 0 {
			statuses = append(statuses, string(node.Status.Phase))
		}
		for _, c := range node.Status.Conditions {
			if c.Status != "True" {
				continue
			}
			statuses = append(statuses, string(c.Type))
		}
		addressesMap := make(map[string]bool)
		var addresses []string
		for _, addr := range node.Status.Addresses {
			if addressesMap[addr.Address] == true {
				continue
			}
			addressesMap[addr.Address] = true
			addresses = append(addresses, addr.Address)
		}

		rows = append(rows, Row{
			colorNode("[node]"),
			colorNode(node.ObjectMeta.Namespace),
			colorNode(node.ObjectMeta.Name),
			colorNode(strings.Join(statuses, " ")),
			colorNode(strings.Join(addresses, " ")),
			colorNode(shortHumanDuration(time.Since(node.CreationTimestamp.Time))),
		})
	}
	ch <- rows
}

func getServices(ch chan Rows, clientset *kubernetes.Clientset) {
	services, err := clientset.Core().Services("").List(v1.ListOptions{})
	if err != nil {
		log.Fatal(err)
	}

	var rows Rows
	for _, service := range services.Items {
		if service.ObjectMeta.Namespace == "kube-system" {
			continue
		}
		var statuses []string
		for _, c := range service.Status.LoadBalancer.Ingress {
			statuses = append(statuses, fmt.Sprintf("%s %s", c.IP, c.Hostname))
		}
		var ports []string
		for _, c := range service.Spec.Ports {
			ports = append(ports, c.Name)
		}
		var ips []string
		for _, ip := range service.Spec.ExternalIPs {
			ips = append(ips, ip)
		}
		if service.Spec.ClusterIP != "" {
			ips = append(ips, service.Spec.ClusterIP)
		}
		rows = append(rows, Row{
			colorService("[service]"),
			colorService(service.ObjectMeta.Namespace),
			colorService(service.ObjectMeta.Name),
			colorService(strings.Join(statuses, ",")),
			colorService(strings.Join(ips, " ") + " " + strings.Join(ports, " ")),
			colorService(shortHumanDuration(time.Since(service.CreationTimestamp.Time))),
		})
	}
	ch <- rows
}

func getDeployments(ch chan Rows, clientset *kubernetes.Clientset) {
	deps, err := clientset.Extensions().Deployments("").List(v1.ListOptions{})
	if err != nil {
		log.Fatal(err)
	}

	var rows Rows
	for _, dep := range deps.Items {
		if dep.ObjectMeta.Namespace == "kube-system" {
			continue
		}
		var statuses []string
		for _, c := range dep.Status.Conditions {
			if c.Status != "True" {
				continue
			}
			statuses = append(statuses, string(c.Type))
		}
		rows = append(rows, Row{
			colorDeployment("[deployment]"),
			colorDeployment(dep.ObjectMeta.Namespace),
			colorDeployment(fmt.Sprintf("%v", dep.ObjectMeta.Name)),
			colorDeployment(fmt.Sprintf("DES=%d CUR=%d AVA=%d %s",
				*dep.Spec.Replicas,
				dep.Status.Replicas,
				dep.Status.AvailableReplicas,
				strings.Join(statuses, " "),
			)),
			colorDeployment(""), // IP
			colorDeployment(shortHumanDuration(time.Since(dep.CreationTimestamp.Time))),
		})
	}
	ch <- rows
}

func getPods(ch chan Rows, clientset *kubernetes.Clientset) {
	pods, err := clientset.Core().Pods("").List(v1.ListOptions{})
	if err != nil {
		log.Fatal(err)
	}

	var rows Rows
	for _, pod := range pods.Items {
		if pod.ObjectMeta.Namespace == "kube-system" {
			continue
		}
		var statuses []string
		statuses = append(statuses, string(pod.Status.Phase))
		for _, c := range pod.Status.Conditions {
			if c.Status != "True" {
				continue
			}
			statuses = append(statuses, string(c.Type))
		}
		rows = append(rows, Row{
			colorPod("[pod]"),
			colorPod(pod.ObjectMeta.Namespace),
			colorPod(fmt.Sprintf("%v", truncate(pod.ObjectMeta.Name))),
			colorPod(strings.Join(statuses, " ")),
			colorPod(pod.Status.PodIP), //pod.Status.HostIP, pod.ObjectMeta.Labels),
			colorPod(shortHumanDuration(time.Since(pod.CreationTimestamp.Time))),
		})
	}
	ch <- rows
}

func render(header Row, rows Rows) {
	for i, row := range rows {
		if len(header) != len(row) {
			log.Fatalf("len(header)=%d != len(row)=%d for row %d", len(header), len(rows), i)
		}
	}
	table := tablewriter.NewWriter(os.Stdout)
	table.SetAutoWrapText(false)
	table.SetHeader(header)
	table.SetHeaderAlignment(tablewriter.ALIGN_LEFT)
	table.SetBorder(false)
	table.SetColumnSeparator("")
	table.SetCenterSeparator("")
	for _, row := range rows {
		table.Append([]string(row))
	}
	table.Render()
}

func truncate(s string) string {
	const max = 20
	const rightLen = 5
	if len(s) < max {
		return s
	}
	return s[0:max-3-rightLen] + "..." + s[len(s)-rightLen:]
}

// shortHumanDuration is copied from
// k8s.io/kubernetes/pkg/kubectl/resource_printer.go
func shortHumanDuration(d time.Duration) string {
	// Allow deviation no more than 2 seconds(excluded) to tolerate machine time
	// inconsistence, it can be considered as almost now.
	if seconds := int(d.Seconds()); seconds < -1 {
		return fmt.Sprintf("<invalid>")
	} else if seconds < 0 {
		return fmt.Sprintf("0s")
	} else if seconds < 60 {
		return fmt.Sprintf("%ds", seconds)
	} else if minutes := int(d.Minutes()); minutes < 60 {
		return fmt.Sprintf("%dm", minutes)
	} else if hours := int(d.Hours()); hours < 24 {
		return fmt.Sprintf("%dh", hours)
	} else if hours < 24*364 {
		return fmt.Sprintf("%dd", hours/24)
	}
	return fmt.Sprintf("%dy", int(d.Hours()/24/365))
}

func clear() {
	cmd := exec.Command("clear")
	cmd.Stdout = os.Stdout
	cmd.Run()
}
