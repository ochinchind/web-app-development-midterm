package main

import (
	"context"
	"log"
	"net"
	"os/signal"
	"syscall"

	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"

	"cargo-transport-service/core/internal/auth"
	"cargo-transport-service/core/internal/config"
	"cargo-transport-service/core/internal/db"
	"cargo-transport-service/core/internal/service"

	pb "cargo-transport-service/core/gen/cargo/v1"
)

var publicMethods = map[string]bool{
	"/cargo.v1.AuthService/Register":                  true,
	"/cargo.v1.AuthService/Login":                     true,
	"/cargo.v1.OrganizationService/GetOrganization":   true,
	"/cargo.v1.OrganizationService/ListOrganizations": true,
	"/grpc.health.v1.Health/Check":                    true,
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}

	pool, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()

	if err := db.Migrate(ctx, pool); err != nil {
		log.Fatal(err)
	}

	jwt := auth.NewManager(cfg.JWTSecret, cfg.JWTTTL)
	server := grpc.NewServer(grpc.UnaryInterceptor(auth.Interceptor(jwt, publicMethods)))

	pb.RegisterAuthServiceServer(server, service.NewAuthService(pool, jwt))
	pb.RegisterOrganizationServiceServer(server, service.NewOrganizationService(pool))
	pb.RegisterVehicleServiceServer(server, service.NewVehicleService(pool))
	pb.RegisterOrderServiceServer(server, service.NewOrderService(pool))
	healthpb.RegisterHealthServer(server, health.NewServer())
	reflection.Register(server)

	lis, err := net.Listen("tcp", ":"+cfg.GRPCPort)
	if err != nil {
		log.Fatal(err)
	}

	go func() {
		<-ctx.Done()
		log.Print("shutting down")
		server.GracefulStop()
	}()

	log.Printf("core gRPC server listening on :%s", cfg.GRPCPort)
	if err := server.Serve(lis); err != nil {
		log.Fatal(err)
	}
}
