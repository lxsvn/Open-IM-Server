package friend

import (
	chat "Open_IM/internal/rpc/msg"
	"Open_IM/pkg/common/config"
	"Open_IM/pkg/common/constant"
	imdb "Open_IM/pkg/common/db/mysql_model/im_mysql_model"
	"Open_IM/pkg/common/log"
	"Open_IM/pkg/common/token_verify"
	"Open_IM/pkg/grpc-etcdv3/getcdv3"
	pbFriend "Open_IM/pkg/proto/friend"
	sdkws "Open_IM/pkg/proto/sdk_ws"
	"Open_IM/pkg/utils"
	"context"
	"google.golang.org/grpc"
	"net"
	"strconv"
	"strings"
)

type friendServer struct {
	rpcPort         int
	rpcRegisterName string
	etcdSchema      string
	etcdAddr        []string
}

func NewFriendServer(port int) *friendServer {
	log.NewPrivateLog("friend")
	return &friendServer{
		rpcPort:         port,
		rpcRegisterName: config.Config.RpcRegisterName.OpenImFriendName,
		etcdSchema:      config.Config.Etcd.EtcdSchema,
		etcdAddr:        config.Config.Etcd.EtcdAddr,
	}
}

func (s *friendServer) Run() {
	log.NewInfo("0", "friendServer run...")

	ip := utils.ServerIP
	registerAddress := ip + ":" + strconv.Itoa(s.rpcPort)
	//listener network
	listener, err := net.Listen("tcp", registerAddress)
	if err != nil {
		log.NewError("0", "Listen failed ", err.Error(), registerAddress)
		return
	}
	log.NewInfo("0", "listen ok ", registerAddress)
	defer listener.Close()
	//grpc server
	srv := grpc.NewServer()
	defer srv.GracefulStop()
	//User friend related services register to etcd
	pbFriend.RegisterFriendServer(srv, s)
	err = getcdv3.RegisterEtcd(s.etcdSchema, strings.Join(s.etcdAddr, ","), ip, s.rpcPort, s.rpcRegisterName, 10)
	if err != nil {
		log.NewError("0", "RegisterEtcd failed ", err.Error(), s.etcdSchema, strings.Join(s.etcdAddr, ","), ip, s.rpcPort, s.rpcRegisterName)
		return
	}
	err = srv.Serve(listener)
	if err != nil {
		log.NewError("0", "Serve failed ", err.Error(), listener)
		return
	}
}

////
//func (s *friendServer) GetFriendsInfo(ctx context.Context, req *pbFriend.GetFriendsInfoReq) (*pbFriend.GetFriendInfoResp, error) {
//	return nil, nil
////	log.NewInfo(req.CommID.OperationID, "GetFriendsInfo args ", req.String())
////	var (
////		isInBlackList int32
////		//	isFriend      int32
////		comment string
////	)
////
////	friendShip, err := imdb.FindFriendRelationshipFromFriend(req.CommID.FromUserID, req.CommID.ToUserID)
////	if err != nil {
////		log.NewError(req.CommID.OperationID, "FindFriendRelationshipFromFriend failed ", err.Error())
////		return &pbFriend.GetFriendInfoResp{ErrCode: constant.ErrSearchUserInfo.ErrCode, ErrMsg: constant.ErrSearchUserInfo.ErrMsg}, nil
////		//	isFriend = constant.FriendFlag
////	}
////	comment = friendShip.Remark
////
////	friendUserInfo, err := imdb.FindUserByUID(req.CommID.ToUserID)
////	if err != nil {
////		log.NewError(req.CommID.OperationID, "FindUserByUID failed ", err.Error())
////		return &pbFriend.GetFriendInfoResp{ErrCode: constant.ErrSearchUserInfo.ErrCode, ErrMsg: constant.ErrSearchUserInfo.ErrMsg}, nil
////	}
////
////	err = imdb.FindRelationshipFromBlackList(req.CommID.FromUserID, req.CommID.ToUserID)
////	if err == nil {
////		isInBlackList = constant.BlackListFlag
////	}
////
////	resp := pbFriend.GetFriendInfoResp{ErrCode: 0, ErrMsg:  "",}
////
////	utils.CopyStructFields(resp.FriendInfoList, friendUserInfo)
////	resp.Data.IsBlack = isInBlackList
////	resp.Data.OwnerUserID = req.CommID.FromUserID
////	resp.Data.Remark = comment
////	resp.Data.CreateTime = friendUserInfo.CreateTime
////
////	log.NewInfo(req.CommID.OperationID, "GetFriendsInfo ok ", resp)
////	return &resp, nil
////
//}

func (s *friendServer) AddBlacklist(ctx context.Context, req *pbFriend.AddBlacklistReq) (*pbFriend.AddBlacklistResp, error) {
	log.NewInfo(req.CommID.OperationID, "AddBlacklist args ", req.String())
	ok := token_verify.CheckAccess(req.CommID.OpUserID, req.CommID.FromUserID)
	if !ok {
		log.NewError(req.CommID.OperationID, "CheckAccess failed ", req.CommID.OpUserID, req.CommID.FromUserID)
	}
	black := imdb.Black{OwnerUserID: req.CommID.FromUserID, BlockUserID: req.CommID.ToUserID}

	err := imdb.InsertInToUserBlackList(black)
	if err != nil {
		log.NewError(req.CommID.OperationID, "InsertInToUserBlackList failed ", err.Error())
		return &pbFriend.AddBlacklistResp{CommonResp: &pbFriend.CommonResp{ErrCode: constant.ErrDB.ErrCode, ErrMsg: constant.ErrDB.ErrMsg}}, nil
	}
	log.NewInfo(req.CommID.OperationID, "InsertInToUserBlackList ok ", req.CommID.FromUserID, req.CommID.ToUserID)
	chat.BlackAddedNotification(req)
	return &pbFriend.AddBlacklistResp{CommonResp: &pbFriend.CommonResp{}}, nil
}

func (s *friendServer) AddFriend(ctx context.Context, req *pbFriend.AddFriendReq) (*pbFriend.AddFriendResp, error) {
	log.NewInfo(req.CommID.OperationID, "AddFriend args ", req.String())
	ok := token_verify.CheckAccess(req.CommID.OpUserID, req.CommID.FromUserID)
	if !ok {
		log.NewError(req.CommID.OperationID, "CheckAccess failed ", req.CommID.OpUserID, req.CommID.FromUserID)
	}
	//Cannot add non-existent users
	if _, err := imdb.GetUserByUserID(req.CommID.ToUserID); err != nil {
		log.NewError(req.CommID.OperationID, "GetUserByUserID failed ", err.Error(), req.CommID.ToUserID)
		return &pbFriend.AddFriendResp{CommonResp: &pbFriend.CommonResp{ErrCode: constant.ErrDB.ErrCode, ErrMsg: constant.ErrDB.ErrMsg}}, nil
	}

	//Establish a latest relationship in the friend request table
	friendRequest := imdb.FriendRequest{ReqMessage: req.ReqMsg}
	utils.CopyStructFields(&friendRequest, req.CommID)
	err := imdb.UpdateFriendApplication(&friendRequest)
	if err != nil {
		log.NewError(req.CommID.OperationID, "UpdateFriendApplication failed ", err.Error())
		return &pbFriend.AddFriendResp{CommonResp: &pbFriend.CommonResp{ErrCode: constant.ErrDB.ErrCode, ErrMsg: constant.ErrDB.ErrMsg}}, nil
	}

	chat.FriendApplicationAddedNotification(req)
	return &pbFriend.AddFriendResp{CommonResp: &pbFriend.CommonResp{}}, nil
}

func (s *friendServer) ImportFriend(ctx context.Context, req *pbFriend.ImportFriendReq) (*pbFriend.ImportFriendResp, error) {
	log.NewInfo(req.OperationID, "ImportFriend failed ", req.String())
	var resp pbFriend.ImportFriendResp
	var c pbFriend.CommonResp

	if !utils.IsContain(req.OpUserID, config.Config.Manager.AppManagerUid) {
		log.NewError(req.OperationID, "not authorized", req.OpUserID)
		c.ErrCode = constant.ErrAccess.ErrCode
		c.ErrMsg = constant.ErrAccess.ErrMsg
		return &pbFriend.ImportFriendResp{CommonResp: &c, FailedFriendUserIDList: req.FriendUserIDList}, nil
	}
	if _, err := imdb.GetUserByUserID(req.FromUserID); err != nil {
		log.NewError(req.OperationID, "FindUserByUID failed ", err.Error(), req.FromUserID)
		c.ErrCode = constant.ErrDB.ErrCode
		c.ErrMsg = "this user not exists,cant not add friend"
		return &pbFriend.ImportFriendResp{CommonResp: &c, FailedFriendUserIDList: req.FriendUserIDList}, nil
	}

	for _, v := range req.FriendUserIDList {
		if _, fErr := imdb.GetUserByUserID(v); fErr != nil {
			c.ErrMsg = "some uid establish failed"
			c.ErrCode = 408
			resp.FailedFriendUserIDList = append(resp.FailedFriendUserIDList, v)
		} else {
			if _, err := imdb.GetFriendRelationshipFromFriend(req.FromUserID, v); err != nil {
				//Establish two single friendship
				toInsertFollow := imdb.Friend{OwnerUserID: req.FromUserID, FriendUserID: v}
				err1 := imdb.InsertToFriend(&toInsertFollow)
				if err1 != nil {
					resp.FailedFriendUserIDList = append(resp.FailedFriendUserIDList, v)
					log.NewError(req.OperationID, "InsertToFriend failed", req.FromUserID, v, err1.Error())
					c.ErrMsg = "some uid establish failed"
					c.ErrCode = 408
					continue
				}
				toInsertFollow = imdb.Friend{OwnerUserID: v, FriendUserID: req.FromUserID}
				err2 := imdb.InsertToFriend(&toInsertFollow)
				if err2 != nil {
					resp.FailedFriendUserIDList = append(resp.FailedFriendUserIDList, v)
					log.NewError(req.OperationID, "InsertToFriend failed", v, req.FromUserID, err2.Error())
					c.ErrMsg = "some uid establish failed"
					c.ErrCode = 408
					continue
				}
				for _, v := range req.FriendUserIDList {
					chat.FriendAddedNotification(req.OperationID, req.OpUserID, req.FromUserID, v)
				}
			}
		}
	}
	resp.CommonResp = &c
	log.NewInfo(req.OperationID, "ImportFriend rpc ok ", resp)
	return &resp, nil
}

//process Friend application
func (s *friendServer) AddFriendResponse(ctx context.Context, req *pbFriend.AddFriendResponseReq) (*pbFriend.AddFriendResponseResp, error) {
	log.NewInfo(req.CommID.OperationID, "AddFriendResponse args ", req.String())

	if !token_verify.CheckAccess(req.CommID.FromUserID, req.CommID.ToUserID) {
		log.NewError(req.CommID.OperationID, "CheckAccess failed ", req.CommID.FromUserID, req.CommID.ToUserID)
		return &pbFriend.AddFriendResponseResp{CommonResp: &pbFriend.CommonResp{ErrCode: constant.ErrAccess.ErrCode, ErrMsg: constant.ErrAccess.ErrMsg}}, nil
	}

	//Check there application before agreeing or refuse to a friend's application
	//req.CommID.FromUserID process req.CommID.ToUserID
	friendRequest, err := imdb.GetFriendApplicationByBothUserID(req.CommID.ToUserID, req.CommID.FromUserID)
	if err != nil {
		log.NewError(req.CommID.OperationID, "GetFriendApplicationByBothUserID failed ", err.Error(), req.CommID.ToUserID, req.CommID.FromUserID)
		return &pbFriend.AddFriendResponseResp{CommonResp: &pbFriend.CommonResp{ErrCode: constant.ErrDB.ErrCode, ErrMsg: constant.ErrDB.ErrMsg}}, nil
	}

	friendRequest.HandleResult = req.Flag
	//Change friend request status flag
	err = imdb.UpdateFriendApplication(friendRequest)
	if err != nil {
		log.NewError(req.CommID.OperationID, "UpdateFriendApplication failed ", err.Error(), friendRequest)
		return &pbFriend.AddFriendResponseResp{CommonResp: &pbFriend.CommonResp{ErrCode: constant.ErrDB.ErrCode, ErrMsg: constant.ErrDB.ErrMsg}}, nil
	}
	log.NewInfo(req.CommID.OperationID, "rpc AddFriendResponse ok")

	//Change the status of the friend request form
	if req.Flag == constant.FriendFlag {
		//Establish friendship after find friend relationship not exists
		_, err := imdb.FindFriendRelationshipFromFriend(req.CommID.FromUserID, req.CommID.ToUserID)
		if err == nil {
			log.NewWarn(req.CommID.OperationID, "FindFriendRelationshipFromFriend exist", req.CommID.FromUserID, req.CommID.ToUserID)
		} else {
			//Establish two single friendship
			err = imdb.InsertToFriend(req.CommID.FromUserID, req.CommID.ToUserID, req.Flag)
			if err != nil {
				log.NewError(req.CommID.OperationID, "InsertToFriend failed ", err.Error(), req.CommID.FromUserID, req.CommID.ToUserID, req.Flag)
				return &pbFriend.AddFriendResponseResp{CommonResp: &pbFriend.CommonResp{ErrCode: constant.ErrDB.ErrCode, ErrMsg: constant.ErrDB.ErrMsg}}, nil
			}
		}

		_, err = imdb.FindFriendRelationshipFromFriend(req.CommID.ToUserID, req.CommID.FromUserID)
		if err == nil {
			log.NewWarn(req.CommID.OperationID, "FindFriendRelationshipFromFriend exist", req.CommID.ToUserID, req.CommID.FromUserID)
			return &pbFriend.AddFriendResponseResp{CommonResp: &pbFriend.CommonResp{}}, nil
		}
		err = imdb.InsertToFriend(req.CommID.ToUserID, req.CommID.FromUserID, req.Flag)
		if err != nil {
			log.NewError(req.CommID.OperationID, "InsertToFriend failed ", err.Error(), req.CommID.FromUserID, req.CommID.ToUserID, req.Flag)
			return &pbFriend.AddFriendResponseResp{CommonResp: &pbFriend.CommonResp{ErrCode: constant.ErrDB.ErrCode, ErrMsg: constant.ErrDB.ErrMsg}}, nil
		}
	}

	chat.FriendApplicationProcessedNotification(req)
	chat.FriendAddedNotification(req.CommID.OperationID, req.CommID.OpUserID, req.CommID.FromUserID, req.CommID.ToUserID)
	return &pbFriend.AddFriendResponseResp{CommonResp: &pbFriend.CommonResp{}}, nil
}

func (s *friendServer) DeleteFriend(ctx context.Context, req *pbFriend.DeleteFriendReq) (*pbFriend.DeleteFriendResp, error) {
	log.NewInfo(req.CommID.OperationID, "DeleteFriend args ", req.String())
	//Parse token, to find current user information
	if !token_verify.CheckAccess(req.CommID.OpUserID, req.CommID.FromUserID) {
		log.NewError(req.CommID.OperationID, "CheckAccess false ", req.CommID.OpUserID, req.CommID.FromUserID)
		return &pbFriend.DeleteFriendResp{CommonResp: &pbFriend.CommonResp{ErrCode: constant.ErrAccess.ErrCode, ErrMsg: constant.ErrAccess.ErrMsg}}, nil
	}

	err := imdb.DeleteSingleFriendInfo(req.CommID.FromUserID, req.CommID.ToUserID)
	if err != nil {
		log.NewError(req.CommID.OperationID, "DeleteSingleFriendInfo failed", err.Error(), req.CommID.FromUserID, req.CommID.ToUserID)
		return &pbFriend.DeleteFriendResp{CommonResp: &pbFriend.CommonResp{ErrCode: constant.ErrAccess.ErrCode, ErrMsg: constant.ErrAccess.ErrMsg}}, nil
	}
	log.NewInfo(req.CommID.OperationID, "DeleteFriend rpc ok")
	chat.FriendDeletedNotification(req)
	return &pbFriend.DeleteFriendResp{CommonResp: &pbFriend.CommonResp{}}, nil
}

func (s *friendServer) GetBlacklist(ctx context.Context, req *pbFriend.GetBlacklistReq) (*pbFriend.GetBlacklistResp, error) {
	log.NewInfo(req.CommID.OperationID, "GetBlacklist args ", req.String())

	//Parse token, to find current user information
	if !token_verify.CheckAccess(req.CommID.OpUserID, req.CommID.FromUserID) {
		log.NewError(req.CommID.OperationID, "CheckAccess failed", req.CommID.OpUserID, req.CommID.FromUserID)
		return &pbFriend.GetBlacklistResp{ErrCode: constant.ErrAccess.ErrCode, ErrMsg: constant.ErrAccess.ErrMsg}, nil
	}

	blackListInfo, err := imdb.GetBlackListByUserID(req.CommID.FromUserID)
	if err != nil {
		log.NewError(req.CommID.OperationID, "GetBlackListByUID failed ", err.Error(), req.CommID.FromUserID)
		return &pbFriend.GetBlacklistResp{ErrCode: constant.ErrDB.ErrCode, ErrMsg: constant.ErrDB.ErrMsg}, nil
	}

	var (
		userInfoList []*sdkws.PublicUserInfo
	)
	for _, blackUser := range blackListInfo {
		var blackUserInfo sdkws.PublicUserInfo
		//Find black user information
		us, err := imdb.GetUserByUserID(blackUser.BlockUserID)
		if err != nil {
			log.NewError(req.CommID.OperationID, "FindUserByUID failed ", err.Error(), blackUser.BlockUserID)
			continue
		}
		utils.CopyStructFields(&blackUserInfo, us)
		userInfoList = append(userInfoList, &blackUserInfo)
	}
	log.NewInfo(req.CommID.OperationID, "rpc GetBlacklist ok")
	return &pbFriend.GetBlacklistResp{BlackUserInfoList: userInfoList}, nil
}

func (s *friendServer) SetFriendComment(ctx context.Context, req *pbFriend.SetFriendCommentReq) (*pbFriend.SetFriendCommentResp, error) {
	log.NewInfo(req.CommID.OperationID, "SetFriendComment args ", req.String())
	//Parse token, to find current user information
	if !token_verify.CheckAccess(req.CommID.OpUserID, req.CommID.FromUserID) {
		log.NewError(req.CommID.OperationID, "CheckAccess false", req.CommID.OpUserID, req.CommID.FromUserID)
		return &pbFriend.SetFriendCommentResp{CommonResp: &pbFriend.CommonResp{ErrCode: constant.ErrAccess.ErrCode, ErrMsg: constant.ErrAccess.ErrMsg}}, nil
	}

	err := imdb.UpdateFriendComment(req.CommID.FromUserID, req.CommID.OpUserID, req.Remark)
	if err != nil {
		log.NewError(req.CommID.OperationID, "UpdateFriendComment failed ", err.Error(), req.CommID.FromUserID, req.CommID.OpUserID, req.Remark)
		return &pbFriend.SetFriendCommentResp{CommonResp: &pbFriend.CommonResp{ErrCode: constant.ErrDB.ErrCode, ErrMsg: constant.ErrDB.ErrMsg}}, nil
	}
	log.NewInfo(req.CommID.OperationID, "rpc SetFriendComment ok")
	chat.FriendInfoChangedNotification(req.CommID.OperationID, req.CommID.OpUserID, req.CommID.FromUserID, req.CommID.ToUserID)
	return &pbFriend.SetFriendCommentResp{CommonResp: &pbFriend.CommonResp{}}, nil
}

func (s *friendServer) RemoveBlacklist(ctx context.Context, req *pbFriend.RemoveBlacklistReq) (*pbFriend.RemoveBlacklistResp, error) {
	log.NewInfo(req.CommID.OperationID, "RemoveBlacklist args ", req.String())
	//Parse token, to find current user information
	if !token_verify.CheckAccess(req.CommID.OpUserID, req.CommID.FromUserID) {
		log.NewError(req.CommID.OperationID, "CheckAccess false", req.CommID.OpUserID, req.CommID.FromUserID)
		return &pbFriend.RemoveBlacklistResp{CommonResp: &pbFriend.CommonResp{ErrCode: constant.ErrAccess.ErrCode, ErrMsg: constant.ErrAccess.ErrMsg}}, nil
	}
	err := imdb.RemoveBlackList(req.CommID.FromUserID, req.CommID.ToUserID)
	if err != nil {
		log.NewError(req.CommID.OperationID, "RemoveBlackList failed", err.Error(), req.CommID.FromUserID, req.CommID.ToUserID)
		return &pbFriend.RemoveBlacklistResp{CommonResp: &pbFriend.CommonResp{ErrCode: constant.ErrAccess.ErrCode, ErrMsg: constant.ErrAccess.ErrMsg}}, nil

	}
	log.NewInfo(req.CommID.OperationID, "rpc RemoveBlacklist ok")
	chat.BlackDeletedNotification(req)
	return &pbFriend.RemoveBlacklistResp{CommonResp: &pbFriend.CommonResp{}}, nil
}

func (s *friendServer) IsInBlackList(ctx context.Context, req *pbFriend.IsInBlackListReq) (*pbFriend.IsInBlackListResp, error) {
	log.NewInfo("IsInBlackList args ", req.String())
	if !token_verify.CheckAccess(req.CommID.OpUserID, req.CommID.FromUserID) {
		log.NewError(req.CommID.OperationID, "CheckAccess false", req.CommID.OpUserID, req.CommID.FromUserID)
		return &pbFriend.IsInBlackListResp{ErrCode: constant.ErrAccess.ErrCode, ErrMsg: constant.ErrAccess.ErrMsg}, nil
	}

	var isInBlacklist = false
	err := imdb.CheckBlack(req.CommID.FromUserID, req.CommID.ToUserID)
	if err == nil {
		isInBlacklist = true
	}
	log.NewInfo(req.CommID.OperationID, "IsInBlackList rpc ok")
	return &pbFriend.IsInBlackListResp{Response: isInBlacklist}, nil
}

func (s *friendServer) IsFriend(ctx context.Context, req *pbFriend.IsFriendReq) (*pbFriend.IsFriendResp, error) {
	log.NewInfo("IsFriend args ", req.String())
	var isFriend bool
	if !token_verify.CheckAccess(req.CommID.OpUserID, req.CommID.FromUserID) {
		log.NewError(req.CommID.OperationID, "CheckAccess false ", req.CommID.OpUserID, req.CommID.FromUserID)
		return &pbFriend.IsFriendResp{ErrCode: constant.ErrAccess.ErrCode, ErrMsg: constant.ErrAccess.ErrMsg}, nil
	}
	_, err := imdb.GetFriendRelationshipFromFriend(req.CommID.FromUserID, req.CommID.ToUserID)
	if err == nil {
		isFriend = true
	} else {
		isFriend = false
	}
	log.NewInfo("IsFriend rpc ok")
	return &pbFriend.IsFriendResp{Response: isFriend}, nil
}

func (s *friendServer) GetFriendList(ctx context.Context, req *pbFriend.GetFriendListReq) (*pbFriend.GetFriendListResp, error) {
	log.NewInfo("GetFriendList args ", req.String())
	var userInfoList []*sdkws.FriendInfo
	//Parse token, to find current user information
	if !token_verify.CheckAccess(req.CommID.OpUserID, req.CommID.FromUserID) {
		log.NewError(req.CommID.OperationID, "CheckAccess false ", req.CommID.OpUserID, req.CommID.FromUserID)
		return &pbFriend.GetFriendListResp{ErrCode: constant.ErrAccess.ErrCode, ErrMsg: constant.ErrAccess.ErrMsg}, nil
	}

	friends, err := imdb.GetUserInfoFromFriend(req.CommID.FromUserID)
	if err != nil {
		log.NewError(req.CommID.OperationID, "FindUserInfoFromFriend failed", err.Error(), req.CommID.FromUserID)
		return &pbFriend.GetFriendListResp{ErrCode: constant.ErrDB.ErrCode, ErrMsg: constant.ErrDB.ErrMsg}, nil
	}
	for _, friendUser := range friends {
		var friendUserInfo sdkws.FriendInfo
		//find user is in blackList
		//	err = imdb.GetRelationshipFromBlackList(req.CommID.FromUserID, friendUser.FriendUserID)
		//if err == nil {
		//	friendUserInfo.IsBlack = constant.BlackListFlag
		//} else {
		//	friendUserInfo.IsBlack = 0
		//}
		//Find user information
		us, err := imdb.GetUserByUserID(friendUser.FriendUserID)
		if err != nil {
			log.NewError(req.CommID.OperationID, "FindUserByUID failed", err.Error(), friendUser.FriendUserID)
			continue
		}
		utils.CopyStructFields(friendUserInfo.FriendUser, us)
		friendUserInfo.Remark = friendUser.Remark
		friendUserInfo.OwnerUserID = req.CommID.FromUserID
		friendUserInfo.CreateTime = friendUser.CreateTime
		userInfoList = append(userInfoList, &friendUserInfo)
	}
	log.NewInfo(req.CommID.OperationID, "rpc GetFriendList ok", pbFriend.GetFriendListResp{FriendInfoList: userInfoList})
	return &pbFriend.GetFriendListResp{FriendInfoList: userInfoList}, nil
}

func (s *friendServer) GetFriendApplyList(ctx context.Context, req *pbFriend.GetFriendApplyListReq) (*pbFriend.GetFriendApplyListResp, error) {
	log.NewInfo(req.CommID.OperationID, "GetFriendApplyList args ", req.String())

	//Parse token, to find current user information
	if !token_verify.CheckAccess(req.CommID.OpUserID, req.CommID.FromUserID) {
		log.NewError(req.CommID.OperationID, "CheckAccess false ", req.CommID.OpUserID, req.CommID.FromUserID)
		return &pbFriend.GetFriendApplyListResp{ErrCode: constant.ErrAccess.ErrCode, ErrMsg: constant.ErrAccess.ErrMsg}, nil
	}

	//	Find the  current user friend applications received
	ApplyUsersInfo, err := imdb.GetReceivedFriendsApplicationListByUserID(req.CommID.FromUserID)
	if err != nil {
		log.NewError(req.CommID.OperationID, "FindFriendsApplyFromFriendReq ", err.Error(), req.CommID.FromUserID)
		return &pbFriend.GetFriendApplyListResp{ErrCode: constant.ErrDB.ErrCode, ErrMsg: constant.ErrDB.ErrMsg}, nil
	}

	var appleUserList []*sdkws.FriendRequest
	for _, applyUserInfo := range ApplyUsersInfo {
		var userInfo sdkws.FriendRequest
		utils.CopyStructFields(&userInfo, applyUserInfo)
		appleUserList = append(appleUserList, &userInfo)

	}
	log.NewInfo(req.CommID.OperationID, "rpc GetFriendApplyList ok", pbFriend.GetFriendApplyListResp{FriendRequestList: appleUserList})
	return &pbFriend.GetFriendApplyListResp{FriendRequestList: appleUserList}, nil
}

func (s *friendServer) GetSelfApplyList(ctx context.Context, req *pbFriend.GetSelfApplyListReq) (*pbFriend.GetSelfApplyListResp, error) {
	log.NewInfo(req.CommID.OperationID, "GetSelfApplyList args ", req.String())
	var selfApplyOtherUserList []*sdkws.FriendRequest
	//Parse token, to find current user information
	if !token_verify.CheckAccess(req.CommID.OpUserID, req.CommID.FromUserID) {
		log.NewError(req.CommID.OperationID, "CheckAccess false ", req.CommID.OpUserID, req.CommID.FromUserID)
		return &pbFriend.GetSelfApplyListResp{ErrCode: constant.ErrAccess.ErrCode, ErrMsg: constant.ErrAccess.ErrMsg}, nil
	}

	//	Find the self add other userinfo
	usersInfo, err := imdb.GetSendFriendApplicationListByUserID(req.CommID.FromUserID)
	if err != nil {
		log.NewError(req.CommID.OperationID, "FindSelfApplyFromFriendReq failed ", err.Error(), req.CommID.FromUserID)
		return &pbFriend.GetSelfApplyListResp{ErrCode: constant.ErrDB.ErrCode, ErrMsg: constant.ErrDB.ErrMsg}, nil
	}
	for _, selfApplyOtherUserInfo := range usersInfo {
		var userInfo sdkws.FriendRequest // pbFriend.ApplyUserInfo
		utils.CopyStructFields(&userInfo, selfApplyOtherUserInfo)
		selfApplyOtherUserList = append(selfApplyOtherUserList, &userInfo)
	}
	log.NewInfo(req.CommID.OperationID, "rpc GetSelfApplyList ok", pbFriend.GetSelfApplyListResp{FriendRequestList: selfApplyOtherUserList})
	return &pbFriend.GetSelfApplyListResp{FriendRequestList: selfApplyOtherUserList}, nil
}