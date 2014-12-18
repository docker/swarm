package cluster

import (
	"crypto/tls"
	"errors"
	"strings"
	"sync"

	log "github.com/Sirupsen/logrus"
	"github.com/docker/swarm/discovery"
	"github.com/samalba/dockerclient"
)

var (
	ErrNodeNotConnected      = errors.New("node is not connected to docker's REST API")
	ErrNodeAlreadyRegistered = errors.New("node was already added to the cluster")
)

type Cluster struct {
	sync.RWMutex
	store         *Store
	tlsConfig     *tls.Config
	eventHandlers []EventHandler
	nodes         map[string]*Node
	containers    map[*Node][]*Container
}

func NewCluster(store *Store, tlsConfig *tls.Config) *Cluster {
	return &Cluster{
		store:      store,
		tlsConfig:  tlsConfig,
		nodes:      make(map[string]*Node),
		containers: make(map[*Node][]*Container),
	}
}

func (c *Cluster) assignVirtualId(container *Container) {
	// Try a reverse lookup of the container into the store to find out if we already did the mapping.
	for _, sc := range c.store.All() {
		if sc.Id == container.Id {
			log.Debugf("Restored container %s (%s)", sc.VirtualId, sc.Id)
			container.VirtualId = sc.VirtualId
			if err := c.store.Replace(sc.VirtualId, container); err != nil {
				log.Errorf("Unable to update state for container %s: %v", container.Id, err)
			}
			return
		}
	}

	// This is an unknown container - generate a VID and store it.
	vid := generateVirtualId()
	log.Debugf("Mapping container %s to VID %s", container.Id, vid)
	container.VirtualId = vid
	if err := c.store.Add(vid, container); err != nil {
		log.Errorf("Unable to save state for container %s: %v", container.Id, err)
	}
}

func (c *Cluster) refreshContainers(node *Node) {
	c.Lock()
	defer c.Unlock()

	c.containers[node] = node.Containers()

	// VID mapping.
	for _, container := range c.containers[node] {
		// Assign virtual ID to containers without any.
		if len(container.VirtualId) == 0 {
			c.assignVirtualId(container)
		}
	}
}

// Deploy a container into a `specific` node on the cluster.
func (c *Cluster) DeployContainer(node *Node, config *dockerclient.ContainerConfig, name string) (*Container, error) {
	container, err := node.Create(config, name, true)
	if err != nil {
		return nil, err
	}
	c.refreshContainers(node)
	return container, nil
}

// Destroys a given `container` from the cluster.
func (c *Cluster) DestroyContainer(container *Container, force bool) error {
	if err := container.Node.Destroy(container, force); err != nil {
		return err
	}
	if err := c.store.Remove(container.VirtualId); err != nil {
		return err
	}
	c.refreshContainers(container.Node)
	return nil
}

func (c *Cluster) Handle(e *Event) error {
	// Refresh the container list for `node` as soon as we receive an event.
	c.refreshContainers(e.Node)

	// Dispatch the event to all the handlers.
	for _, eventHandler := range c.eventHandlers {
		if err := eventHandler.Handle(e); err != nil {
			log.Error(err)
		}
	}
	return nil
}

// Register a node within the cluster. The node must have been already
// initialized.
func (c *Cluster) AddNode(n *Node) error {
	if !n.IsConnected() {
		return ErrNodeNotConnected
	}

	c.Lock()
	if _, exists := c.nodes[n.ID]; exists {
		c.Unlock()
		return ErrNodeAlreadyRegistered
	}
	// Register the node.
	c.nodes[n.ID] = n
	c.Unlock()

	c.refreshContainers(n)
	return n.Events(c)
}

func (c *Cluster) UpdateNodes(nodes []*discovery.Node) {
	for _, addr := range nodes {
		go func(node *discovery.Node) {
			if c.Node(node.String()) == nil {
				n := NewNode(node.String())
				if err := n.Connect(c.tlsConfig); err != nil {
					log.Error(err)
					return
				}
				if err := c.AddNode(n); err != nil {
					log.Error(err)
					return
				}
			}
		}(addr)
	}
}

// Containers returns all the containers in the cluster.
func (c *Cluster) Containers() []*Container {
	c.RLock()
	defer c.RUnlock()

	out := []*Container{}
	for _, containers := range c.containers {
		for _, c := range containers {
			out = append(out, c)
		}
	}

	return out
}

// Container returns the container with ID in the cluster
func (c *Cluster) Container(IdOrName string) *Container {
	// Abort immediately if the name is empty.
	if len(IdOrName) == 0 {
		return nil
	}

	c.RLock()
	defer c.RUnlock()
	for _, container := range c.Containers() {
		// Match ID prefix.
		if strings.HasPrefix(container.VirtualId, IdOrName) {
			return container
		}

		// Match name, /name or engine/name.
		for _, name := range container.Names {
			if name == IdOrName || name == "/"+IdOrName || container.Node.ID+name == IdOrName || container.Node.Name+name == IdOrName {
				return container
			}
		}
	}

	return nil
}

// Nodes returns the list of nodes in the cluster
func (c *Cluster) Nodes() []*Node {
	nodes := []*Node{}
	c.RLock()
	for _, node := range c.nodes {
		nodes = append(nodes, node)
	}
	c.RUnlock()
	return nodes
}

func (c *Cluster) Node(addr string) *Node {
	for _, node := range c.nodes {
		if node.Addr == addr {
			return node
		}
	}
	return nil
}

func (c *Cluster) Events(h EventHandler) error {
	c.eventHandlers = append(c.eventHandlers, h)
	return nil
}
