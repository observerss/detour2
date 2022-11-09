package deploy

import (
	"detour/common"
	"detour/logger"
)

func DeployServer(conf *common.DeployConfig) {
	if conf.Remove {
		RemoveServer(conf)
		return
	}

	logger.Info.Println("deploy on aliyun...")
	fc, err := NewClient(conf)
	if err != nil {
		logger.Error.Fatal(err)
	}

	err = fc.FindService()
	if err != nil {
		logger.Info.Println("create service...")
		fc.CreateService()
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
		logger.Error.Fatal(err)
	}

	_, err = fc.GetTrigger()
	if err != nil {
		logger.Info.Println("create trigger...")
		err = fc.CreateTrigger()
		if err != nil {
			logger.Error.Fatal(err)
		}
	}
	url, _ := fc.GetHTTPURL()
	ws, _ := fc.GetWebsocketURL()
	logger.Info.Printf("deploy ok.\n    url = %s\n    ws = %s", url, ws)
}

func RemoveServer(conf *common.DeployConfig) {
	logger.Info.Println("remove on aliyun...")
	fc, err := NewClient(conf)
	if err != nil {
		logger.Error.Fatal(err)
	}

	logger.Info.Println("remove trigger...")
	fc.DeleteTrigger()

	logger.Info.Println("remove function...")
	fc.DeleteFunction()

	logger.Info.Println("remove service...")
	fc.DeleteService()

	logger.Info.Println("remove ok.")
}
