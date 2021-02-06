package pktesting

import (
	"github.com/peake100/gRPEAKEC-go/pkservices"
	"github.com/stretchr/testify/suite"
)



type InnerSuite interface {
	Assertions
	suite.TestingSuite
}

// ManagerSuite offers some
type ManagerSuite struct {
	InnerSuite
	Manager *pkservices.Manager
}

// SetupSuite implements suite.SetupAllSuite, and starts the manager running in it's own
// routine, and blocks until it can ping the gRPC server if there are any gRPC services.
func (m *ManagerSuite) SetupSuite() {
	if !m.NotNil(m.Manager, "manager is set") {
		m.FailNow("manager is nil")
	}

	go func() {
		err := m.Manager.Run()
		m.NoError(err, "run manager")
	}()

	tester := m.Manager.Test(m.T())

	hasGrpc := false
	for _, service := range tester.Services() {
		_, ok := service.(pkservices.GrpcService)
		if ok {
			hasGrpc = true
			break
		}
	}

	// If the manager is not managing gRPC services, we can return.
	if !hasGrpc {
		return
	}

	ctx, cancel := New3SecondCtx()
	defer cancel()

	tester.PingGrpcServer(ctx)

	// If our inner suite is a setup suite, run it's setup.
	if innerSetupSuite, ok := m.InnerSuite.(suite.SetupAllSuite) ; ok {
		innerSetupSuite.SetupSuite()
	}
}

// TearDownSuite implements suite.TearDownAllSuite and shuts down the manager.
func (m *ManagerSuite) TearDownSuite() {
	if m.Manager != nil {
		// Signal shutdown of the manager.
		m.Manager.StartShutdown()
		// Block until shutdown complete.
		m.Manager.WaitForShutdown()
	}

	// If our inner suite is a teardown suite, run it's setup.
	if innerTearDownSuite, ok := m.InnerSuite.(suite.TearDownAllSuite) ; ok {
		innerTearDownSuite.TearDownSuite()
	}
}

// NewManagerSuite returns a new ManagerSuite with the given innerSuite. If a nil
// innerSuite is passed, then a new *suite.Suite will be used.
func NewManagerSuite(innerSuite InnerSuite) ManagerSuite {
	if innerSuite == nil {
		innerSuite = new(suite.Suite)
	}
	return ManagerSuite{
		InnerSuite:	innerSuite,
	}
}