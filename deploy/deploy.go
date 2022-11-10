package deploy

import (
	"fmt"
	"strings"

	"github.com/observerss/detour2/common"

	openapi "github.com/alibabacloud-go/darabonba-openapi/v2/client"
	fc_open20210406 "github.com/alibabacloud-go/fc-open-20210406/client"
	"github.com/alibabacloud-go/tea/tea"
)

type Client struct {
	Client *fc_open20210406.Client
	Conf   *common.DeployConfig
}

func NewClient(conf *common.DeployConfig) (_result *Client, _err error) {
	config := &openapi.Config{
		AccessKeyId:     &conf.AccessKeyId,
		AccessKeySecret: &conf.AccessKeySecret,
	}
	endpoint := fmt.Sprintf("%s.%s.fc.aliyuncs.com", conf.AccountId, conf.Region)
	config.Endpoint = &endpoint
	client := &Client{Conf: conf}
	cli, err := fc_open20210406.NewClient(config)
	if err != nil {
		return nil, err
	}
	client.Client = cli
	return client, nil
}

func (c *Client) DeleteFunction() error {
	_, err := c.Client.DeleteFunction(&c.Conf.ServiceName, &c.Conf.FunctionName)
	return err
}

func (c *Client) FindFunction() error {
	request := &fc_open20210406.GetFunctionRequest{}
	_, err := c.Client.GetFunction(&c.Conf.ServiceName, &c.Conf.FunctionName, request)
	if err != nil {
		return err
	}
	return nil
}

func (c *Client) CreateFunction() error {
	request := &fc_open20210406.CreateFunctionRequest{
		CaPort: tea.Int32(3811),
		Cpu:    tea.Float32(0.1),
		CustomContainerConfig: &fc_open20210406.CustomContainerConfig{
			AccelerationType: tea.String("Default"),
			Args:             tea.String(""),
			Command:          tea.String(fmt.Sprintf(`["./detour","server","-p","%s"]`, c.Conf.Password)),
			Image:            tea.String(c.Conf.Image),
			WebServerMode:    tea.Bool(true),
		},
		Description:           tea.String(""),
		DiskSize:              tea.Int32(512),
		EnvironmentVariables:  map[string]*string{"TZ": tea.String("Asia/Shanghai")},
		FunctionName:          &c.Conf.FunctionName,
		Handler:               tea.String("index.handler"),
		InitializationTimeout: tea.Int32(3),
		Initializer:           tea.String(""),
		InstanceConcurrency:   tea.Int32(100),
		InstanceType:          tea.String("e1"),
		MemorySize:            tea.Int32(128),
		Runtime:               tea.String("custom-container"),
		Timeout:               tea.Int32(600),
	}
	_, err := c.Client.CreateFunction(&c.Conf.ServiceName, request)
	if err != nil {
		return err
	}
	return nil
}

func (c *Client) UpdateFunction() error {
	request := &fc_open20210406.UpdateFunctionRequest{
		CaPort: tea.Int32(3811),
		Cpu:    tea.Float32(0.05),
		CustomContainerConfig: &fc_open20210406.CustomContainerConfig{
			AccelerationType: tea.String("Default"),
			Args:             tea.String(""),
			Command:          tea.String(fmt.Sprintf(`["./detour","server","-p","%s"]`, c.Conf.Password)),
			Image:            tea.String(c.Conf.Image),
			WebServerMode:    tea.Bool(true),
		},
		Description:           tea.String(""),
		DiskSize:              tea.Int32(512),
		EnvironmentVariables:  map[string]*string{"TZ": tea.String("Asia/Shanghai")},
		Handler:               tea.String("index.handler"),
		InitializationTimeout: tea.Int32(3),
		Initializer:           tea.String(""),
		InstanceConcurrency:   tea.Int32(100),
		InstanceType:          tea.String("e1"),
		MemorySize:            tea.Int32(128),
		Runtime:               tea.String("custom-container"),
		Timeout:               tea.Int32(600),
	}
	_, err := c.Client.UpdateFunction(&c.Conf.ServiceName, &c.Conf.FunctionName, request)
	if err != nil {
		return err
	}
	return nil
}

func (c *Client) DeleteTrigger() error {
	_, err := c.Client.DeleteTrigger(&c.Conf.ServiceName, &c.Conf.FunctionName, &c.Conf.TriggerName)
	return err
}

func (c *Client) GetTrigger() (*fc_open20210406.GetTriggerResponseBody, error) {
	response, err := c.Client.GetTrigger(&c.Conf.ServiceName, &c.Conf.FunctionName, &c.Conf.TriggerName)
	if err != nil {
		return nil, err
	}
	return response.Body, nil
}

func (c *Client) GetWebsocketURL() (string, error) {
	trigger, err := c.GetTrigger()
	if err != nil {
		return "", err
	}
	return strings.Replace(*trigger.UrlInternet, "https://", "wss://", 1) + "/ws", nil
}

func (c *Client) GetHTTPURL() (string, error) {
	trigger, err := c.GetTrigger()
	if err != nil {
		return "", err
	}
	return *trigger.UrlInternet, nil
}

func (c *Client) CreateTrigger() error {
	request := &fc_open20210406.CreateTriggerRequest{
		Description:   tea.String(""),
		TriggerConfig: tea.String(`{"methods":["GET","POST","PUT","DELETE"],"authType":"anonymous","disableURLInternet":false}`),
		TriggerName:   &c.Conf.TriggerName,
		TriggerType:   tea.String("http"),
	}
	_, err := c.Client.CreateTrigger(&c.Conf.ServiceName, &c.Conf.FunctionName, request)
	return err
}

func (c *Client) DeleteService() error {
	_, err := c.Client.DeleteService(&c.Conf.ServiceName)
	return err
}

func (c *Client) FindService() error {
	_, err := c.Client.GetService(&c.Conf.ServiceName, &fc_open20210406.GetServiceRequest{})
	return err
}

func (c *Client) CreateService() error {
	request := &fc_open20210406.CreateServiceRequest{
		Description:    tea.String(""),
		InternetAccess: tea.Bool(true),
		ServiceName:    &c.Conf.ServiceName,
	}
	_, err := c.Client.CreateService(request)
	return err
}
