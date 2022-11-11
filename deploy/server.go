package deploy

import (
	"github.com/observerss/detour2/common"
	"github.com/observerss/detour2/logger"
)

func DeployServer(conf *common.DeployConfig) error {
	if conf.Remove {
		return RemoveServer(conf)
	}

	logger.Info.Println("deploy on aliyun...")
	fc, err := NewClient(conf)
	if err != nil {
		return err
	}

	err = fc.FindService()
	if err != nil {
		logger.Info.Println("create service...")
		err = fc.CreateService()
		if err != nil {
			return err
		}
	}

	err = fc.FindFunction()
	if err != nil {
		logger.Info.Println("create function...")
		err = fc.CreateFunction()
	} else {
		logger.Info.Println("update function...")
		err = fc.UpdateFunction()
	}

	if err != nil {
		return err
	}

	_, err = fc.GetTrigger()
	if err != nil {
		logger.Info.Println("create trigger...")
		err = fc.CreateTrigger()
		if err != nil {
			return err
		}
	}
	url, _ := fc.GetHTTPURL()
	ws, _ := fc.GetWebsocketURL()
	logger.Info.Printf("deploy ok.\n    url = %s\n    ws = %s", url, ws)
	return nil
}

func RemoveServer(conf *common.DeployConfig) error {
	logger.Info.Println("remove on aliyun...")
	fc, err := NewClient(conf)
	if err != nil {
		logger.Error.Fatal(err)
	}

	logger.Info.Println("remove trigger...")
	err = fc.DeleteTrigger()
	if err != nil {
		return err
	}

	logger.Info.Println("remove function...")
	err = fc.DeleteFunction()
	if err != nil {
		return err
	}

	logger.Info.Println("remove service...")
	err = fc.DeleteService()
	if err != nil {
		return err
	}

	logger.Info.Println("remove ok.")
	return nil
}
