// Copyright 2014 loolgame Author. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
package room

import (
	"github.com/liangdas/mqant/gate"
)

var (
	Uninitialized = 0 //未初始化
	Initialized   = 1 //已初始化的
	Active        = 2 //活跃状态
	Paused        = 3 //暂停状态
	Stoped        = 4 //已停止状态
)

type BaseTable interface {
	Options() Options
	TableId() string

	OnCreate()  //可以进行一些初始化的工作在table第一次被创建的时候调用
	OnStart()   //table创建完成，但还不可与用户交互，无法接收用户消息 开始：onCreate()->onStart() onStop()->onRestart()->onStart()
	OnRestart() //在table停止后，在再次启动之前被调用 重启  onStop()->onRestart()
	OnResume()  //取得控制权，可接受用户输入。 恢复：onCreate()->onStart()->onResume() onPause()->onResume() onStop()->onRestart()->onStart()->onResume()
	OnPause()   //table内暂停，可接收用户消息，此方法主要用来将未保存的变化进行持久化，停止游戏时钟等 暂停：onStart()->onPause()
	OnStop()    //当table不再提供服务时调用此方法，将无法再接收用户消息 停止:onPause()->onStop()
	OnDestroy() //在table销毁时调用 销毁：onPause()->onStop()->onDestroy()
	OnTimeOut() //当table超时了

	State() int   //uninitialized active paused stoped destroyed
	Runing() bool //table是否在Runing中,只要在Runing中就能接收和处理消息
	Create()
	Start()
	Stop()
	Pause()   //暂停
	Resume()  //恢复
	Restart() //重新开始
	Finish()  //停止table

	Register(id string, f interface{})
	SetReceive(receive QueueReceive)
	PutQueue(_func string, params ...interface{}) error
	ExecuteEvent(arge interface{})
}

type BasePlayer interface {
	IsBind() bool
	Bind(session gate.Session) BasePlayer
	/**
	玩家主动发请求时触发
	*/
	OnRequest(session gate.Session)
	/**
	服务器主动发送消息给玩家时触发
	*/
	OnResponse(session gate.Session)
	/*
		服务器跟玩家最后一次成功通信时间
	*/
	GetLastReqResDate() int64
	Body() interface{}
	SetBody(body interface{})
	Session() gate.Session
	Type() string
}
