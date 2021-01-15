package pktesting

import (
	"github.com/peake100/gRPEAKEC-go/pkservices"
	"github.com/stretchr/testify/suite"
)

// ManagerSuite offers some
type ManagerSuite struct {
	suite.Suite
	Manager *pkservices.Manager
}

func (m *ManagerSuite) TearDownSuite() {
	if m.Manager != nil {
		m.Manager.StartShutdown()
	}
}

func (m *ManagerSuite) SetupSuite() {
	if !m.NotNil(m.Manager, "manager is set") {
		m.FailNow("manager is nil")
	}

	go func() {
		err := m.Manager.Run()
		m.NoError(err, "run manager")
	}()
}
