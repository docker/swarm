package cluster

import (
	"fmt"
	"io/ioutil"
	"os"
	"testing"

	"github.com/samalba/dockerclient"
	"github.com/samalba/dockerclient/mockclient"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func createNode(t *testing.T, ID string, containers ...dockerclient.Container) *Node {
	node := NewNode(ID)
	node.Name = ID

	assert.False(t, node.IsConnected())

	client := mockclient.NewMockClient()
	client.On("Info").Return(mockInfo, nil)
	client.On("ListContainers", true, false, "").Return(containers, nil)
	client.On("InspectContainer", mock.Anything).Return(
		&dockerclient.ContainerInfo{
			Config: &dockerclient.ContainerConfig{CpuShares: 100},
		}, nil)
	client.On("StartMonitorEvents", mock.Anything, mock.Anything).Return()

	assert.NoError(t, node.connectClient(client))
	assert.True(t, node.IsConnected())
	node.ID = ID

	return node
}

func TestAddNode(t *testing.T) {
	dir, err := ioutil.TempDir("", "store-test")
	assert.NoError(t, err)
	defer assert.NoError(t, os.RemoveAll(dir))
	c := NewCluster(NewStore(dir), nil)

	assert.Equal(t, len(c.Nodes()), 0)
	assert.Nil(t, c.Node("test"))
	assert.Nil(t, c.Node("test2"))

	assert.NoError(t, c.AddNode(createNode(t, "test")))
	assert.Equal(t, len(c.Nodes()), 1)
	assert.NotNil(t, c.Node("test"))

	assert.Error(t, c.AddNode(createNode(t, "test")))
	assert.Equal(t, len(c.Nodes()), 1)
	assert.NotNil(t, c.Node("test"))

	assert.NoError(t, c.AddNode(createNode(t, "test2")))
	assert.Equal(t, len(c.Nodes()), 2)
	assert.NotNil(t, c.Node("test2"))
}

func TestContainerLookup(t *testing.T) {
	dir, err := ioutil.TempDir("", "store-test")
	assert.NoError(t, err)
	defer assert.NoError(t, os.RemoveAll(dir))
	c := NewCluster(NewStore(dir), nil)

	container := dockerclient.Container{
		Id:    "container-id",
		Names: []string{"/container-name1", "/container-name2"},
	}
	node := createNode(t, "test-node", container)
	assert.NoError(t, c.AddNode(node))

	// Invalid lookup
	assert.Nil(t, c.Container("invalid-id"))
	assert.Nil(t, c.Container(""))
	// Container name lookup.
	assert.NotNil(t, c.Container("container-name1"))
	assert.NotNil(t, c.Container("container-name2"))
	// Container node/name matching.
	assert.NotNil(t, c.Container("test-node/container-name1"))
	assert.NotNil(t, c.Container("test-node/container-name2"))

	// Container ID lookup.
	vid := c.Container("container-name1").VirtualId
	assert.NotNil(t, c.Container(vid))
	// Container ID prefix lookup.
	assert.NotNil(t, c.Container(vid[0:3]))
}

func TestContainerNodeMapping(t *testing.T) {
	// Create a test node.
	node := NewNode("test")

	client := mockclient.NewMockClient()
	client.On("Info").Return(mockInfo, nil)
	client.On("StartMonitorEvents", mock.Anything, mock.Anything).Return()

	// The client will return one container at first, then a second one will appear.
	client.On("ListContainers", true, false, "").Return([]dockerclient.Container{{Id: "one"}}, nil).Once()
	client.On("InspectContainer", "one").Return(&dockerclient.ContainerInfo{Config: &dockerclient.ContainerConfig{CpuShares: 100}}, nil).Once()
	client.On("ListContainers", true, false, fmt.Sprintf("{%q:[%q]}", "id", "two")).Return([]dockerclient.Container{{Id: "two"}}, nil).Once()
	client.On("InspectContainer", "two").Return(&dockerclient.ContainerInfo{Config: &dockerclient.ContainerConfig{CpuShares: 100}}, nil).Once()

	assert.NoError(t, node.connectClient(client))
	assert.True(t, node.IsConnected())

	// Create a test cluster.
	dir, err := ioutil.TempDir("", "store-test")
	assert.NoError(t, err)
	defer assert.NoError(t, os.RemoveAll(dir))
	c := NewCluster(NewStore(dir), nil)
	assert.NoError(t, c.AddNode(node))

	// Ensure that the cluster picked up the already existing container from
	// our test node.
	assert.Len(t, c.Containers(), 1)
	assert.Equal(t, c.Containers()[0].Id, "one")
	// And make sure a virtual id was generated.
	assert.NotEmpty(t, c.Containers()[0].VirtualId)

	// Simulate a manually created container on the node.
	node.handler(&dockerclient.Event{Id: "two", Status: "created"})
	// Make sure it got picked up...
	assert.Len(t, node.Containers(), 2)
	// ...And that it has a virtual id
	assert.NotEmpty(t, c.Containers()[0].VirtualId)
	assert.NotEmpty(t, c.Containers()[1].VirtualId)

	client.Mock.AssertExpectations(t)
}

func TestDeployContainer(t *testing.T) {
	// Create a test node.
	node := createNode(t, "test")

	// Create a test cluster.
	dir, err := ioutil.TempDir("", "store-test")
	assert.NoError(t, err)
	defer assert.NoError(t, os.RemoveAll(dir))
	c := NewCluster(NewStore(dir), nil)
	assert.NoError(t, c.AddNode(node))

	// Fake dockerclient calls to deploy a container.
	client := node.client.(*mockclient.MockClient)
	client.On("CreateContainer", mock.Anything, mock.Anything).Return("id", nil).Once()
	client.On("ListContainers", true, false, mock.Anything).Return([]dockerclient.Container{{Id: "id"}}, nil).Once()
	client.On("InspectContainer", "id").Return(&dockerclient.ContainerInfo{Config: &dockerclient.ContainerConfig{CpuShares: 100}}, nil).Once()

	// Ensure the container gets deployed and a virtual id generated.
	container, err := c.DeployContainer(node, &dockerclient.ContainerConfig{}, "name")
	assert.NoError(t, err)
	assert.Equal(t, container.Id, "id")
	assert.NotEmpty(t, container.VirtualId)
}
