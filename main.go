package main

import (
	"context"
	"log"
	"net"
	"net/http"

	"github.com/TranQuocToan1996/redislearn/config"
	"github.com/TranQuocToan1996/redislearn/controllers"
	"github.com/TranQuocToan1996/redislearn/gapi"
	"github.com/TranQuocToan1996/redislearn/pb"
	"github.com/TranQuocToan1996/redislearn/routes"
	"github.com/TranQuocToan1996/redislearn/services"
	"github.com/TranQuocToan1996/redislearn/utils"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readpref"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

var (
	server = gin.Default()
	ctx    = context.Background()

	mongoclient    *mongo.Client
	authCollection *mongo.Collection

	redisclient *redis.Client

	userService services.UserService
	authService services.AuthService

	UserController      controllers.UserController
	UserRouteController routes.UserRouteController
	AuthController      controllers.AuthController
	AuthRouteController routes.AuthRouteController

	postService         services.PostService
	PostController      controllers.PostController
	postCollection      *mongo.Collection
	PostRouteController routes.PostRouteController

	temp, _ = utils.ParseTemplateDir("./templates")
)

func init() {
	cfg, err := config.LoadConfig(".")
	if err != nil {
		cfg, err = config.LoadConfig("../../")
		if err != nil {
			log.Fatal("Could not load config", err)
		}
	}

	// Connect to MongoDB
	mongoconnOpt := options.Client().ApplyURI(cfg.DBUri)
	mongoclient, err := mongo.Connect(ctx, mongoconnOpt)

	if err != nil {
		panic(err)
	}

	if err := mongoclient.Ping(ctx, readpref.Primary()); err != nil {
		panic(err)
	}

	log.Println("MongoDB successfully connected...")

	redisclient = redis.NewClient(&redis.Options{
		Addr: cfg.RedisUri,
	})

	if _, err := redisclient.Ping().Result(); err != nil {
		panic(err)
	}

	err = redisclient.Set("test", "Welcome to Golang with Redis and MongoDB",
		0).Err()
	if err != nil {
		panic(err)
	}

	log.Println("Redis client connected successfully...")

	// Collections
	authCollection = mongoclient.Database("golang_mongodb").Collection("users")
	userService = services.NewUserServiceImpl(authCollection, ctx)
	authService = services.NewAuthService(authCollection, ctx)
	AuthController = controllers.NewAuthController(authService, userService, ctx, authCollection, temp)
	AuthRouteController = routes.NewAuthRouteController(AuthController)

	UserController = controllers.NewUserController(userService)
	UserRouteController = routes.NewRouteUserController(UserController)

	err = services.NewJWT(cfg)
	if err != nil {
		panic(err)
	}

	postCollection = mongoclient.Database("golang_mongodb").Collection("posts")
	postService = services.NewPostService(postCollection, ctx)
	PostController = controllers.NewPostController(postService)
	PostRouteController = routes.NewPostControllerRoute(PostController)

}

func main() {
	cfg, err := config.LoadConfig(".")

	if err != nil {
		cfg, err = config.LoadConfig("../../")
		if err != nil {
			log.Fatal("Could not load config", err)
		}
	}

	defer mongoclient.Disconnect(ctx)

	// startGinServer(config)
	startGrpcServer(cfg)
}

func startGinServer(config config.Config) {
	value, err := redisclient.Get("test").Result()

	if err == redis.Nil {
		log.Println("key: test does not exist")
	} else if err != nil {
		panic(err)
	}

	corsConfig := cors.DefaultConfig()
	corsConfig.AllowOrigins = []string{config.Origin}
	corsConfig.AllowCredentials = true

	server.Use(cors.New(corsConfig))

	router := server.Group("/api")
	router.GET("/healthchecker", func(ctx *gin.Context) {
		ctx.JSON(http.StatusOK, gin.H{"status": "success", "message": value})
	})

	AuthRouteController.AuthRoute(router, userService)
	UserRouteController.UserRoute(router, userService)
	log.Fatal(server.Run(":" + config.Port))
}

func startGrpcServer(config config.Config) {
	authServer, err := gapi.NewGrpcAuthServer(config, authService, userService, authCollection)
	if err != nil {
		log.Fatal("cannot create grpc authServer: ", err)
	}

	userServer, err := gapi.NewGrpcUserServer(config, userService, authCollection)
	if err != nil {
		log.Fatal("cannot create grpc userServer: ", err)
	}

	grpcServer := grpc.NewServer()

	pb.RegisterAuthServiceServer(grpcServer, authServer)
	pb.RegisterUserServiceServer(grpcServer, userServer)
	reflection.Register(grpcServer)

	listener, err := net.Listen("tcp", config.GrpcServerAddress)
	if err != nil {
		log.Fatal("cannot create grpc server: ", err)
	}

	log.Printf("start gRPC server on %s", listener.Addr().String())
	err = grpcServer.Serve(listener)
	if err != nil {
		log.Fatal("cannot create grpc server: ", err)
	}
}
