// article
package models

import (
	"github.com/ginuerzh/sports/errors"
	"labix.org/v2/mgo"
	"labix.org/v2/mgo/bson"
	"labix.org/v2/mgo/txn"
	"log"
	"strings"
	"time"
)

func init() {
	ensureIndex(articleColl, "author")
	ensureIndex(articleColl, "-pub_time")
}

type Segment struct {
	ContentType string `bson:"seg_type" json:"seg_type"`
	ContentText string `bson:"seg_content" json:"seg_content"`
}

type Article struct {
	Id     bson.ObjectId `bson:"_id,omitempty"`
	Parent string        `bson:",omitempty"`
	Author string
	//Title    string `bson:",omitempty"`
	//Image    string `bson:",omitempty"`
	Contents []Segment
	PubTime  time.Time `bson:"pub_time"`

	Views       []string `bson:",omitempty"`
	Thumbs      []string `bson:",omitempty"`
	Reviews     []string `bson:",omitempty"`
	Rewards     []string `bson:",omitempty"`
	TotalReward int64    `bson:"total_reward"`
	Tags        []string `bson:",omitempty"`
}

func (this *Article) Cover() (text string, image string) {
	for _, seg := range this.Contents {
		if len(text) > 0 && len(image) > 0 {
			break
		}
		if len(text) == 0 && strings.ToUpper(seg.ContentType) == "TEXT" {
			text = seg.ContentText
		}
		if len(image) == 0 && strings.ToUpper(seg.ContentType) == "IMAGE" {
			image = seg.ContentText
		}
	}
	return
}

func FindArticles(ids ...string) (articles []Article, err error) {
	var oid []interface{}
	for _, id := range ids {
		oid = append(oid, bson.ObjectIdHex(id))
	}

	if e := findIds(articleColl, oid, &articles); e != nil {
		err = errors.NewError(errors.DbError, e.Error())
	}
	return
}

func (this *Article) findOne(query interface{}) (bool, error) {
	var articles []Article

	err := search(articleColl, query, nil, 0, 1, nil, nil, &articles)
	if err != nil {
		return false, errors.NewError(errors.DbError, err.Error())
	}
	if len(articles) > 0 {
		*this = articles[0]
	}

	return len(articles) > 0, nil
}

func (this *Article) FindById(id string) (bool, error) {
	if !bson.IsObjectIdHex(id) {
		return false, nil
	}
	return this.findOne(bson.M{"_id": bson.ObjectIdHex(id)})
}

func (this *Article) Save() error {
	this.Id = bson.NewObjectId()
	if len(this.Parent) == 0 {
		if err := save(articleColl, this, true); err != nil {
			return errors.NewError(errors.DbError, err.Error())
		}
		return nil
	}

	if !bson.IsObjectIdHex(this.Parent) {
		return errors.NewError(errors.InvalidMsgError)
	}

	f := func(c *mgo.Collection) error {
		runner := txn.NewRunner(c)
		ops := []txn.Op{
			{
				C:      articleColl,
				Id:     this.Id,
				Assert: txn.DocMissing,
				Insert: this,
			},
			{
				C:      articleColl,
				Id:     bson.ObjectIdHex(this.Parent),
				Assert: txn.DocExists,
				Update: bson.M{
					"$addToSet": bson.M{
						"reviews": this.Id.Hex(),
					},
				},
			},
		}

		return runner.Run(ops, bson.NewObjectId(), nil)
	}

	if err := withCollection("comment_tx", &mgo.Safe{}, f); err != nil {
		log.Println(err)
		return errors.NewError(errors.DbError, err.Error())
	}
	return nil
}

func (this *Article) RemoveId() error {
	if err := removeId(articleColl, this.Id, true); err != nil {
		if e, ok := err.(*mgo.LastError); ok {
			return errors.NewError(errors.DbError, e.Error())
		}
	}
	return nil
}

func (this *Article) Remove() error {
	find, err := this.findOne(bson.M{"author": this.Author, "_id": this.Id})
	if !find {
		return err
	}

	if len(this.Parent) == 0 {
		if err := removeId(articleColl, this.Id.Hex(), true); err != nil {
			if e, ok := err.(*mgo.LastError); ok {
				return errors.NewError(errors.DbError, e.Error())
			}
		}
		return nil
	}

	f := func(c *mgo.Collection) error {
		runner := txn.NewRunner(c)
		ops := []txn.Op{
			{
				C:      articleColl,
				Id:     this.Id,
				Remove: true,
			},
			{
				C:  articleColl,
				Id: bson.ObjectIdHex(this.Parent),
				Update: bson.M{
					"$pull": bson.M{
						"reviews": this.Id.Hex(),
					},
				},
			},
		}

		return runner.Run(ops, bson.NewObjectId(), nil)
	}
	if err := withCollection("comment_tx", &mgo.Safe{}, f); err != nil {
		log.Println(err)
		return errors.NewError(errors.DbError, err.Error())
	}
	return nil
}

func (article *Article) Update() error {
	m := bson.M{}

	if len(article.Author) > 0 {
		m["author"] = article.Author
	}
	if len(article.Contents) > 0 {
		m["contents"] = article.Contents
	}
	if len(article.Tags) > 0 {
		m["tags"] = article.Tags
	}
	if article.PubTime.Unix() > 0 {
		m["pub_time"] = article.PubTime
	}

	change := bson.M{
		"$set": m,
	}

	if err := updateId(articleColl, article.Id, change, true); err != nil {
		return errors.NewError(errors.DbError, err.Error())
	}
	return nil
}

func (this *Article) SetThumb(userid string, thumb bool) error {

	var m bson.M

	if thumb {
		m = bson.M{
			"$addToSet": bson.M{
				"thumbs": userid,
			},
		}
	} else {
		m = bson.M{
			"$pull": bson.M{
				"thumbs": userid,
			},
		}
	}

	if err := updateId(articleColl, this.Id, m, true); err != nil {
		return errors.NewError(errors.DbError, err.Error())
	}

	return nil
}

func (this *Article) IsThumbed(userid string) (bool, error) {
	count := 0
	err := search(articleColl, bson.M{"_id": this.Id, "thumbs": userid}, nil, 0, 0, nil, &count, nil)
	if err != nil {
		return false, errors.NewError(errors.DbError, err.Error())
	}
	return count > 0, nil
}

func articlePagingFunc(c *mgo.Collection, first, last string, args ...interface{}) (query bson.M, err error) {
	article := &Article{}

	if bson.IsObjectIdHex(first) {
		if err := c.FindId(bson.ObjectIdHex(first)).One(article); err != nil {
			return nil, err
		}
		query = bson.M{
			"pub_time": bson.M{
				"$gte": article.PubTime,
			},
			"_id": bson.M{
				"$ne": article.Id,
			},
		}
	} else if bson.IsObjectIdHex(last) {
		if err := c.FindId(bson.ObjectIdHex(last)).One(article); err != nil {
			return nil, err
		}
		query = bson.M{
			"pub_time": bson.M{
				"$lte": article.PubTime,
			},
			"_id": bson.M{
				"$ne": article.Id,
			},
		}
	}

	return
}

func GetArticles(tag string, paging *Paging) (int, []Article, error) {
	var articles []Article
	total := 0

	selector := bson.M{
		"parent": nil,
	}
	if len(tag) > 0 {
		selector["tags"] = tag
	}

	pageUp := false
	sortFields := []string{"-pub_time"}
	if len(paging.First) > 0 {
		pageUp = true
		sortFields = []string{"pub_time"}
	}

	if err := psearch(articleColl, selector, nil,
		sortFields, nil, &articles, articlePagingFunc, paging); err != nil {
		e := errors.NewError(errors.DbError, err.Error())
		if err == mgo.ErrNotFound {
			e = errors.NewError(errors.NotFoundError, err.Error())
		}
		return total, nil, e
	}

	paging.First = ""
	paging.Last = ""
	paging.Count = 0
	if len(articles) > 0 {
		if pageUp {
			for i := 0; i < len(articles)/2; i++ {
				t := articles[i]
				articles[i] = articles[len(articles)-i-1]
				articles[len(articles)-i-1] = t
			}
		}
		paging.First = articles[0].Id.Hex()
		paging.Last = articles[len(articles)-1].Id.Hex()
		paging.Count = total
	}

	return total, articles, nil
}

func (this *Article) CommentCount() (count int) {
	search(articleColl, bson.M{"parent": this.Id.Hex()}, nil, 0, 0, nil, &count, nil)
	return
}

func (this *Article) Comments(paging *Paging) (int, []Article, error) {
	var articles []Article
	total := 0

	pageUp := false
	sortFields := []string{"-pub_time"}
	if len(paging.First) > 0 {
		pageUp = true
		sortFields = []string{"pub_time"}
	}

	if err := psearch(articleColl, bson.M{"parent": this.Id.Hex()}, nil,
		sortFields, &total, &articles, articlePagingFunc, paging); err != nil {
		e := errors.NewError(errors.DbError, err.Error())
		if err == mgo.ErrNotFound {
			e = errors.NewError(errors.NotFoundError, err.Error())
		}
		return total, nil, e
	}

	paging.First = ""
	paging.Last = ""
	paging.Count = 0
	if len(articles) > 0 {
		if pageUp {
			for i := 0; i < len(articles)/2; i++ {
				t := articles[i]
				articles[i] = articles[len(articles)-i-1]
				articles[len(articles)-i-1] = t
			}
		}
		paging.First = articles[0].Id.Hex()
		paging.Last = articles[len(articles)-1].Id.Hex()
		paging.Count = total
	}

	return total, articles, nil
}

func (this *Article) AdminComments(pageIndex, pageCount int) (total int, articles []Article, err error) {
	err = search(articleColl, bson.M{"parent": this.Id.Hex()}, nil,
		pageIndex*pageCount, pageCount, []string{"-pub_time"}, &total, &articles)
	return
}

func (this *Article) Reward(userid string, amount int64) error {
	change := mgo.Change{
		Update: bson.M{
			"$addToSet": bson.M{
				"rewards": userid,
			},
			"$inc": bson.M{
				"total_reward": amount,
			},
		},
		ReturnNew: true,
	}
	_, err := apply(articleColl, bson.M{"_id": this.Id}, change, this)

	this.TotalReward += amount

	return err
}

func PostCount(start, end time.Time) int {
	c, _ := count(articleColl, bson.M{"pub_time": bson.M{"$gte": start, "$lt": end}})
	return c
}

func SearchArticle(keyword string, paging *Paging) (int, []Article, error) {
	var articles []Article
	total := 0

	query := bson.M{
		"contents.seg_content": bson.M{
			"$regex":   keyword,
			"$options": "i",
		},
	}

	pageUp := false
	sortFields := []string{"-pub_time"}
	if len(paging.First) > 0 {
		pageUp = true
		sortFields = []string{"pub_time"}
	}

	if err := psearch(articleColl, query, nil, sortFields, &total, &articles,
		articlePagingFunc, paging); err != nil {
		if err != mgo.ErrNotFound {
			return total, nil, errors.NewError(errors.DbError, err.Error())
		}
	}

	paging.First = ""
	paging.Last = ""
	paging.Count = 0
	if len(articles) > 0 {
		if pageUp {
			for i := 0; i < len(articles)/2; i++ {
				t := articles[i]
				articles[i] = articles[len(articles)-i-1]
				articles[len(articles)-i-1] = t
			}
		}
		paging.First = articles[0].Id.Hex()
		paging.Last = articles[len(articles)-1].Id.Hex()
		paging.Count = total
	}

	return total, articles, nil
}

func AdminSearchArticle(keyword string, tag string,
	pageIndex, pageCount int) (total int, articles []Article, err error) {
	query := bson.M{}

	if len(keyword) == 0 {
		query = bson.M{
			"parent": nil,
			"tags":   tag,
		}
	} else {
		query = bson.M{
			"parent": nil,
			"contents.seg_content": bson.M{
				"$regex":   keyword,
				"$options": "i",
			},
		}
		if len(tag) > 0 {
			query["tags"] = tag
		}
	}

	err = search(articleColl, query, nil,
		pageIndex*pageCount, pageCount, []string{"-pub_time"}, &total, &articles)
	return
}

func ArticleList(sort string, pageIndex, pageCount int) (total int, articles []Article, err error) {
	switch sort {
	case "pubtime":
		sort = "pub_time"
	case "-pubtime":
		sort = "-pub_time"
	default:
		sort = "-pub_time"
	}
	err = search(articleColl, bson.M{"parent": nil}, nil,
		pageIndex*pageCount, pageCount, []string{sort}, &total, &articles)
	return
}
