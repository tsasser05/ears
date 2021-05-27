// Code generated by moq; DO NOT EDIT.
// github.com/matryer/moq

package receiver

import (
	"context"
	"sync"
)

// Ensure, that HasherMock does implement Hasher.
// If this is not the case, regenerate this file with moq.
var _ Hasher = &HasherMock{}

// HasherMock is a mock implementation of Hasher.
//
// 	func TestSomethingThatUsesHasher(t *testing.T) {
//
// 		// make and configure a mocked Hasher
// 		mockedHasher := &HasherMock{
// 			ReceiverHashFunc: func(config interface{}) (string, error) {
// 				panic("mock out the ReceiverHash method")
// 			},
// 		}
//
// 		// use mockedHasher in code that requires Hasher
// 		// and then make assertions.
//
// 	}
type HasherMock struct {
	// ReceiverHashFunc mocks the ReceiverHash method.
	ReceiverHashFunc func(config interface{}) (string, error)

	// calls tracks calls to the methods.
	calls struct {
		// ReceiverHash holds details about calls to the ReceiverHash method.
		ReceiverHash []struct {
			// Config is the config argument value.
			Config interface{}
		}
	}
	lockReceiverHash sync.RWMutex
}

// ReceiverHash calls ReceiverHashFunc.
func (mock *HasherMock) ReceiverHash(config interface{}) (string, error) {
	if mock.ReceiverHashFunc == nil {
		panic("HasherMock.ReceiverHashFunc: method is nil but Hasher.ReceiverHash was just called")
	}
	callInfo := struct {
		Config interface{}
	}{
		Config: config,
	}
	mock.lockReceiverHash.Lock()
	mock.calls.ReceiverHash = append(mock.calls.ReceiverHash, callInfo)
	mock.lockReceiverHash.Unlock()
	return mock.ReceiverHashFunc(config)
}

// ReceiverHashCalls gets all the calls that were made to ReceiverHash.
// Check the length with:
//     len(mockedHasher.ReceiverHashCalls())
func (mock *HasherMock) ReceiverHashCalls() []struct {
	Config interface{}
} {
	var calls []struct {
		Config interface{}
	}
	mock.lockReceiverHash.RLock()
	calls = mock.calls.ReceiverHash
	mock.lockReceiverHash.RUnlock()
	return calls
}

// Ensure, that NewReceivererMock does implement NewReceiverer.
// If this is not the case, regenerate this file with moq.
var _ NewReceiverer = &NewReceivererMock{}

// NewReceivererMock is a mock implementation of NewReceiverer.
//
// 	func TestSomethingThatUsesNewReceiverer(t *testing.T) {
//
// 		// make and configure a mocked NewReceiverer
// 		mockedNewReceiverer := &NewReceivererMock{
// 			NewReceiverFunc: func(config interface{}) (Receiver, error) {
// 				panic("mock out the NewReceiver method")
// 			},
// 			ReceiverHashFunc: func(config interface{}) (string, error) {
// 				panic("mock out the ReceiverHash method")
// 			},
// 		}
//
// 		// use mockedNewReceiverer in code that requires NewReceiverer
// 		// and then make assertions.
//
// 	}
type NewReceivererMock struct {
	// NewReceiverFunc mocks the NewReceiver method.
	NewReceiverFunc func(config interface{}) (Receiver, error)

	// ReceiverHashFunc mocks the ReceiverHash method.
	ReceiverHashFunc func(config interface{}) (string, error)

	// calls tracks calls to the methods.
	calls struct {
		// NewReceiver holds details about calls to the NewReceiver method.
		NewReceiver []struct {
			// Config is the config argument value.
			Config interface{}
		}
		// ReceiverHash holds details about calls to the ReceiverHash method.
		ReceiverHash []struct {
			// Config is the config argument value.
			Config interface{}
		}
	}
	lockNewReceiver  sync.RWMutex
	lockReceiverHash sync.RWMutex
}

// NewReceiver calls NewReceiverFunc.
func (mock *NewReceivererMock) NewReceiver(config interface{}) (Receiver, error) {
	if mock.NewReceiverFunc == nil {
		panic("NewReceivererMock.NewReceiverFunc: method is nil but NewReceiverer.NewReceiver was just called")
	}
	callInfo := struct {
		Config interface{}
	}{
		Config: config,
	}
	mock.lockNewReceiver.Lock()
	mock.calls.NewReceiver = append(mock.calls.NewReceiver, callInfo)
	mock.lockNewReceiver.Unlock()
	return mock.NewReceiverFunc(config)
}

// NewReceiverCalls gets all the calls that were made to NewReceiver.
// Check the length with:
//     len(mockedNewReceiverer.NewReceiverCalls())
func (mock *NewReceivererMock) NewReceiverCalls() []struct {
	Config interface{}
} {
	var calls []struct {
		Config interface{}
	}
	mock.lockNewReceiver.RLock()
	calls = mock.calls.NewReceiver
	mock.lockNewReceiver.RUnlock()
	return calls
}

// ReceiverHash calls ReceiverHashFunc.
func (mock *NewReceivererMock) ReceiverHash(config interface{}) (string, error) {
	if mock.ReceiverHashFunc == nil {
		panic("NewReceivererMock.ReceiverHashFunc: method is nil but NewReceiverer.ReceiverHash was just called")
	}
	callInfo := struct {
		Config interface{}
	}{
		Config: config,
	}
	mock.lockReceiverHash.Lock()
	mock.calls.ReceiverHash = append(mock.calls.ReceiverHash, callInfo)
	mock.lockReceiverHash.Unlock()
	return mock.ReceiverHashFunc(config)
}

// ReceiverHashCalls gets all the calls that were made to ReceiverHash.
// Check the length with:
//     len(mockedNewReceiverer.ReceiverHashCalls())
func (mock *NewReceivererMock) ReceiverHashCalls() []struct {
	Config interface{}
} {
	var calls []struct {
		Config interface{}
	}
	mock.lockReceiverHash.RLock()
	calls = mock.calls.ReceiverHash
	mock.lockReceiverHash.RUnlock()
	return calls
}

// Ensure, that ReceiverMock does implement Receiver.
// If this is not the case, regenerate this file with moq.
var _ Receiver = &ReceiverMock{}

// ReceiverMock is a mock implementation of Receiver.
//
// 	func TestSomethingThatUsesReceiver(t *testing.T) {
//
// 		// make and configure a mocked Receiver
// 		mockedReceiver := &ReceiverMock{
// 			ConfigFunc: func() interface{} {
// 				panic("mock out the Config method")
// 			},
// 			NameFunc: func() string {
// 				panic("mock out the Name method")
// 			},
// 			PluginFunc: func() string {
// 				panic("mock out the Plugin method")
// 			},
// 			ReceiveFunc: func(next NextFn) error {
// 				panic("mock out the Receive method")
// 			},
// 			StopReceivingFunc: func(ctx context.Context) error {
// 				panic("mock out the StopReceiving method")
// 			},
// 		}
//
// 		// use mockedReceiver in code that requires Receiver
// 		// and then make assertions.
//
// 	}
type ReceiverMock struct {
	// ConfigFunc mocks the Config method.
	ConfigFunc func() interface{}

	// NameFunc mocks the Name method.
	NameFunc func() string

	// PluginFunc mocks the Plugin method.
	PluginFunc func() string

	// ReceiveFunc mocks the Receive method.
	ReceiveFunc func(next NextFn) error

	// StopReceivingFunc mocks the StopReceiving method.
	StopReceivingFunc func(ctx context.Context) error

	// calls tracks calls to the methods.
	calls struct {
		// Config holds details about calls to the Config method.
		Config []struct {
		}
		// Name holds details about calls to the Name method.
		Name []struct {
		}
		// Plugin holds details about calls to the Plugin method.
		Plugin []struct {
		}
		// Receive holds details about calls to the Receive method.
		Receive []struct {
			// Next is the next argument value.
			Next NextFn
		}
		// StopReceiving holds details about calls to the StopReceiving method.
		StopReceiving []struct {
			// Ctx is the ctx argument value.
			Ctx context.Context
		}
	}
	lockConfig        sync.RWMutex
	lockName          sync.RWMutex
	lockPlugin        sync.RWMutex
	lockReceive       sync.RWMutex
	lockStopReceiving sync.RWMutex
}

// Config calls ConfigFunc.
func (mock *ReceiverMock) Config() interface{} {
	if mock.ConfigFunc == nil {
		panic("ReceiverMock.ConfigFunc: method is nil but Receiver.Config was just called")
	}
	callInfo := struct {
	}{}
	mock.lockConfig.Lock()
	mock.calls.Config = append(mock.calls.Config, callInfo)
	mock.lockConfig.Unlock()
	return mock.ConfigFunc()
}

// ConfigCalls gets all the calls that were made to Config.
// Check the length with:
//     len(mockedReceiver.ConfigCalls())
func (mock *ReceiverMock) ConfigCalls() []struct {
} {
	var calls []struct {
	}
	mock.lockConfig.RLock()
	calls = mock.calls.Config
	mock.lockConfig.RUnlock()
	return calls
}

// Name calls NameFunc.
func (mock *ReceiverMock) Name() string {
	if mock.NameFunc == nil {
		panic("ReceiverMock.NameFunc: method is nil but Receiver.Name was just called")
	}
	callInfo := struct {
	}{}
	mock.lockName.Lock()
	mock.calls.Name = append(mock.calls.Name, callInfo)
	mock.lockName.Unlock()
	return mock.NameFunc()
}

// NameCalls gets all the calls that were made to Name.
// Check the length with:
//     len(mockedReceiver.NameCalls())
func (mock *ReceiverMock) NameCalls() []struct {
} {
	var calls []struct {
	}
	mock.lockName.RLock()
	calls = mock.calls.Name
	mock.lockName.RUnlock()
	return calls
}

// Plugin calls PluginFunc.
func (mock *ReceiverMock) Plugin() string {
	if mock.PluginFunc == nil {
		panic("ReceiverMock.PluginFunc: method is nil but Receiver.Plugin was just called")
	}
	callInfo := struct {
	}{}
	mock.lockPlugin.Lock()
	mock.calls.Plugin = append(mock.calls.Plugin, callInfo)
	mock.lockPlugin.Unlock()
	return mock.PluginFunc()
}

// PluginCalls gets all the calls that were made to Plugin.
// Check the length with:
//     len(mockedReceiver.PluginCalls())
func (mock *ReceiverMock) PluginCalls() []struct {
} {
	var calls []struct {
	}
	mock.lockPlugin.RLock()
	calls = mock.calls.Plugin
	mock.lockPlugin.RUnlock()
	return calls
}

// Receive calls ReceiveFunc.
func (mock *ReceiverMock) Receive(next NextFn) error {
	if mock.ReceiveFunc == nil {
		panic("ReceiverMock.ReceiveFunc: method is nil but Receiver.Receive was just called")
	}
	callInfo := struct {
		Next NextFn
	}{
		Next: next,
	}
	mock.lockReceive.Lock()
	mock.calls.Receive = append(mock.calls.Receive, callInfo)
	mock.lockReceive.Unlock()
	return mock.ReceiveFunc(next)
}

// ReceiveCalls gets all the calls that were made to Receive.
// Check the length with:
//     len(mockedReceiver.ReceiveCalls())
func (mock *ReceiverMock) ReceiveCalls() []struct {
	Next NextFn
} {
	var calls []struct {
		Next NextFn
	}
	mock.lockReceive.RLock()
	calls = mock.calls.Receive
	mock.lockReceive.RUnlock()
	return calls
}

// StopReceiving calls StopReceivingFunc.
func (mock *ReceiverMock) StopReceiving(ctx context.Context) error {
	if mock.StopReceivingFunc == nil {
		panic("ReceiverMock.StopReceivingFunc: method is nil but Receiver.StopReceiving was just called")
	}
	callInfo := struct {
		Ctx context.Context
	}{
		Ctx: ctx,
	}
	mock.lockStopReceiving.Lock()
	mock.calls.StopReceiving = append(mock.calls.StopReceiving, callInfo)
	mock.lockStopReceiving.Unlock()
	return mock.StopReceivingFunc(ctx)
}

// StopReceivingCalls gets all the calls that were made to StopReceiving.
// Check the length with:
//     len(mockedReceiver.StopReceivingCalls())
func (mock *ReceiverMock) StopReceivingCalls() []struct {
	Ctx context.Context
} {
	var calls []struct {
		Ctx context.Context
	}
	mock.lockStopReceiving.RLock()
	calls = mock.calls.StopReceiving
	mock.lockStopReceiving.RUnlock()
	return calls
}
