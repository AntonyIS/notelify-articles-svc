package dynamodb

import (
	"errors"
	"fmt"
	"log"

	appConfig "github.com/AntonyIS/notelify-articles-service/config"
	"github.com/AntonyIS/notelify-articles-service/internal/adapters/logger"
	"github.com/AntonyIS/notelify-articles-service/internal/core/domain"
	"github.com/AntonyIS/notelify-articles-service/internal/core/ports"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/dynamodb"
	"github.com/aws/aws-sdk-go/service/dynamodb/dynamodbattribute"
	"github.com/aws/aws-sdk-go/service/dynamodb/expression"
)

type dynamodbClient struct {
	client    dynamodb.DynamoDB
	tablename string
}

func NewDynamoDBClient(c appConfig.Config, logger logger.LoggerType) ports.ArticleRepository {
	creds := credentials.NewStaticCredentials(c.AWS_ACCESS_KEY, c.AWS_SECRET_KEY, "")
	sess := session.Must(session.NewSession(&aws.Config{
		Region:      aws.String(c.AWS_DEFAULT_REGION),
		Credentials: creds,
	}))
	return &dynamodbClient{
		client:    *dynamodb.New(sess),
		tablename: c.ContentTable,
	}
}

func (db dynamodbClient) CreateArticle(article *domain.Article) (*domain.Article, error) {
	entityParsed, err := dynamodbattribute.MarshalMap(article)
	if err != nil {
		return nil, err
	}
	input := &dynamodb.PutItemInput{
		Item:      entityParsed,
		TableName: aws.String(db.tablename),
	}

	_, err = db.client.PutItem(input)

	if err != nil {
		return nil, err
	}

	return article, nil
}

func (db dynamodbClient) GetArticleByID(article_id string) (*domain.Article, error) {
	result, err := db.client.GetItem(&dynamodb.GetItemInput{
		TableName: aws.String(db.tablename),
		Key: map[string]*dynamodb.AttributeValue{
			"article_id": {
				S: aws.String(article_id),
			},
		},
	})
	if err != nil {
		return &domain.Article{}, err
	}
	if result.Item == nil {
		msg := fmt.Sprintf("Article with id [ %s ] not found", article_id)
		return &domain.Article{}, errors.New(msg)
	}
	var article domain.Article
	err = dynamodbattribute.UnmarshalMap(result.Item, &article)
	if err != nil {
		return &domain.Article{}, err
	}

	return &article, nil
}

func (db dynamodbClient) GetArticlesByAuthor(author_id string) (*[]domain.Article, error) {
	articles, err := db.GetArticles()
	if err != nil {
		return nil, err
	}

	authorArticles := []domain.Article{}
	for _, article := range *articles {
		if article.Author.ID == author_id {
			authorArticles = append(authorArticles, article)
		}
	}
	return &authorArticles, nil
}

func (db dynamodbClient) GetArticlesByTag(tag string) (*[]domain.Article, error) {
	filterExpression := "contains(Tags, :tag)"
	expressionAttributeValues := map[string]*dynamodb.AttributeValue{
		":tag": {
			S: aws.String(tag),
		},
	}

	indexName := "TagsIndex" // Replace with your actual GSI name.
	// Specify the query parameters.
	queryInput := &dynamodb.QueryInput{
		TableName:                 aws.String(db.tablename),
		IndexName:                 aws.String(indexName),
		KeyConditionExpression:    aws.String("Tag = :tag"), // Assuming "Tag" is the index partition key.
		FilterExpression:          aws.String(filterExpression),
		ExpressionAttributeValues: expressionAttributeValues,
	}

	// Execute the query.
	result, err := db.client.Query(queryInput)
	if err != nil {
		log.Fatalf("Query error: %v", err)
	}

	// Process the query results (list of articles matching the tag).
	articles := []domain.Article{}
	for _, item := range result.Items {
		// You can unmarshal the DynamoDB item into your Article struct.
		var article domain.Article
		err := dynamodbattribute.UnmarshalMap(item, &article)
		if err != nil {
			log.Fatalf("Error unmarshaling item: %v", err)
		}

		articles = append(articles, article)
	}
	return &articles, nil
}

func (db dynamodbClient) GetArticles() (*[]domain.Article, error) {
	articles := []domain.Article{}
	filt := expression.Name("ArticleID").AttributeNotExists()
	proj := expression.NamesList(
		expression.Name("article_id"),
		expression.Name("title"),
		expression.Name("subtitle"),
		expression.Name("introduction"),
		expression.Name("body"),
		expression.Name("tags"),
		expression.Name("publish_date"),
		expression.Name("author_info"),
	)
	expr, err := expression.NewBuilder().WithFilter(filt).WithProjection(proj).Build()
	if err != nil {
		return nil, err
	}
	params := &dynamodb.ScanInput{
		ExpressionAttributeNames:  expr.Names(),
		ExpressionAttributeValues: expr.Values(),
		FilterExpression:          expr.Filter(),
		ProjectionExpression:      expr.Projection(),
		TableName:                 aws.String(db.tablename),
	}
	result, err := db.client.Scan(params)

	if err != nil {
		return nil, err
	}

	for _, item := range result.Items {
		var article domain.Article

		err = dynamodbattribute.UnmarshalMap(item, &article)
		if err != nil {
			return nil, err
		}

		articles = append(articles, article)

	}
	return &articles, nil
}

func (db dynamodbClient) UpdateArticle(article *domain.Article) (*domain.Article, error) {
	entityParsed, err := dynamodbattribute.MarshalMap(article)
	if err != nil {
		return nil, err
	}

	input := &dynamodb.PutItemInput{
		Item:      entityParsed,
		TableName: aws.String(db.tablename),
	}

	_, err = db.client.PutItem(input)
	if err != nil {
		return nil, err
	}

	return db.GetArticleByID(article.ArticleID)

}

func (db dynamodbClient) DeleteArticle(article_id string) error {
	input := &dynamodb.DeleteItemInput{
		Key: map[string]*dynamodb.AttributeValue{
			"article_id": {
				S: aws.String(article_id),
			},
		},
		TableName: aws.String(db.tablename),
	}

	res, err := db.client.DeleteItem(input)
	if res == nil {
		return err
	}
	if err != nil {
		return err
	}
	return nil
}

func (db dynamodbClient) DeleteArticleAll() error {
	articles, err := db.GetArticles()
	if err != nil {
		return err
	}

	for _, article := range *articles {
		db.DeleteArticle(article.ArticleID)
	}
	return nil
}
