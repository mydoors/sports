// chat
package admin

import (
	"github.com/ginuerzh/sports/errors"
	"github.com/ginuerzh/sports/models"
	"github.com/martini-contrib/binding"
	"gopkg.in/go-martini/martini.v1"
	"labix.org/v2/mgo/bson"
	"net/http"
	"time"
)

func BindChatApi(m *martini.ClassicMartini) {
	m.Get("/admin/chat/timeline", binding.Form(chatlistForm{}), adminErrorHandler, chatlistHandler)
	m.Post("/admin/chat/delete", binding.Json(delChatForm{}), adminErrorHandler, delChatHandler)
	m.Post("/admin/chat/send", binding.Json(chatSendForm{}), adminErrorHandler, chatSendHandler)
}

type message struct {
	Id       string           `json:"message_id"`
	From     string           `json:"from"`
	To       string           `json:"to"`
	Time     int64            `json:"time"`
	Contents []models.MsgBody `json:"contents"`
}

func convertMsg(msg *models.Message) *message {
	return &message{
		Id:       msg.Id.Hex(),
		From:     msg.From,
		To:       msg.To,
		Time:     msg.Time.Unix(),
		Contents: msg.Body,
	}
}

type chatlistForm struct {
	From     string `form:"from"`
	To       string `form:"to"`
	FromTime int64  `form:"from_time"`
	ToTime   int64  `form:"to_time"`
	AdminPaging
	Token string `form:"access_token" binding:"required"`
}

func chatlistHandler(w http.ResponseWriter, redis *models.RedisLogger, form chatlistForm) {
	user := redis.OnlineUser(form.Token)
	if user == nil {
		writeResponse(w, errors.NewError(errors.AccessError))
		return
	}

	u := &models.User{Id: form.From}
	paging := &models.Paging{First: form.Pre, Last: form.Next, Count: form.Count}
	total, msgs, _ := u.Messages(form.To, paging)

	list := make([]*message, len(msgs))
	for i, _ := range msgs {
		list[i] = convertMsg(&msgs[i])
	}

	resp := map[string]interface{}{
		"messages":     list,
		"prev_cursor":  paging.First,
		"next_cursor":  paging.Last,
		"total_number": total,
	}

	writeResponse(w, resp)
}

type delChatForm struct {
	Id       string `json:"message_id"`
	From     string `json:"from"`
	To       string `json:"to"`
	FromTime int64  `json:"from_time"`
	ToTime   int64  `json:"to_time"`
	Token    string `json:"access_token" binding:"required"`
}

func delChatHandler(w http.ResponseWriter, redis *models.RedisLogger, form delChatForm) {
	user := redis.OnlineUser(form.Token)
	if user == nil {
		writeResponse(w, errors.NewError(errors.AccessError))
		return
	}

	msg := &models.Message{}

	if bson.IsObjectIdHex(form.Id) {
		msg.Id = bson.ObjectIdHex(form.Id)
		if err := msg.RemoveId(); err != nil {
			writeResponse(w, err)
		} else {
			writeResponse(w, map[string]int{"count": 1})
		}
		return
	}

	var start, end time.Time
	if form.FromTime == 0 {
		start = time.Unix(0, 0)
	} else {
		start = time.Unix(form.FromTime, 0)
	}
	if form.ToTime == 0 {
		end = time.Now()
	} else {
		end = time.Unix(form.ToTime, 0)
	}

	count, err := msg.Delete(form.From, form.To, start, end)
	if err != nil {
		writeResponse(w, err)
		return
	}
	writeResponse(w, map[string]int{"count": count})
}

type chatSendForm struct {
	From     string           `json:"from"`
	To       string           `json:"to"`
	Contents []models.MsgBody `json:"contents"`
	Time     int64            `json:"time"`
	Token    string           `json:"access_token" binding:"required"`
}

func chatSendHandler(w http.ResponseWriter, redis *models.RedisLogger, form chatSendForm) {
	user := redis.OnlineUser(form.Token)
	if user == nil {
		writeResponse(w, errors.NewError(errors.AccessError))
		return
	}

	t := time.Now()
	if form.Time > 0 {
		t = time.Unix(form.Time, 0)
	}

	msg := &models.Message{
		From: form.From,
		To:   form.To,
		Type: "chat",
		Body: form.Contents,
		Time: t,
	}
	if err := msg.Save(); err != nil {
		writeResponse(w, err)
		return
	}
	writeResponse(w, map[string]string{"message_id": msg.Id.Hex()})
}