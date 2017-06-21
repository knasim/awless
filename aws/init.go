/*
Copyright 2017 WALLIX

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package aws

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	awssdk "github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/credentials/stscreds"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/wallix/awless/aws/config"
	"github.com/wallix/awless/cloud"
	"github.com/wallix/awless/logger"
	"github.com/wallix/awless/template/driver"
)

var (
	AccessService, InfraService, StorageService, MessagingService, DnsService, LambdaService, MonitoringService, CdnService, CloudformationService cloud.Service
)

func InitServices(conf map[string]interface{}, log *logger.Logger) error {
	awsconf := config(conf)
	region := awsconf.region()
	if region == "" {
		return errors.New("empty AWS region. Set it with `awless config set aws.region`")
	}

	sess, err := initAWSSession(region, awsconf.profile(), log)
	if err != nil {
		return err
	}

	AccessService = NewAccess(sess, awsconf, log)
	InfraService = NewInfra(sess, awsconf, log)
	StorageService = NewStorage(sess, awsconf, log)
	MessagingService = NewMessaging(sess, awsconf, log)
	DnsService = NewDns(sess, awsconf, log)
	LambdaService = NewLambda(sess, awsconf, log)
	MonitoringService = NewMonitoring(sess, awsconf, log)
	CdnService = NewCdn(sess, awsconf, log)
	CloudformationService = NewCloudformation(sess, awsconf, log)

	cloud.ServiceRegistry[InfraService.Name()] = InfraService
	cloud.ServiceRegistry[AccessService.Name()] = AccessService
	cloud.ServiceRegistry[StorageService.Name()] = StorageService
	cloud.ServiceRegistry[MessagingService.Name()] = MessagingService
	cloud.ServiceRegistry[DnsService.Name()] = DnsService
	cloud.ServiceRegistry[LambdaService.Name()] = LambdaService
	cloud.ServiceRegistry[MonitoringService.Name()] = MonitoringService
	cloud.ServiceRegistry[CdnService.Name()] = CdnService
	cloud.ServiceRegistry[CloudformationService.Name()] = CloudformationService

	return nil
}

func NewDriver(region, profile string, log ...*logger.Logger) (driver.Driver, error) {
	if !awsconfig.IsValidRegion(region) {
		return nil, fmt.Errorf("invalid region '%s' provided", region)
	}

	drivLog := logger.DiscardLogger
	if len(log) > 0 {
		drivLog = log[0]
	}

	sess, err := initAWSSession(region, profile, drivLog)
	if err != nil {
		return nil, err
	}

	awsconf := config(
		map[string]interface{}{"aws.region": region, "aws.profile": profile},
	)

	var drivers []driver.Driver
	drivers = append(drivers, NewAccess(sess, awsconf, drivLog).Drivers()...)
	drivers = append(drivers, NewInfra(sess, awsconf, drivLog).Drivers()...)
	drivers = append(drivers, NewStorage(sess, awsconf, drivLog).Drivers()...)
	drivers = append(drivers, NewMessaging(sess, awsconf, drivLog).Drivers()...)
	drivers = append(drivers, NewDns(sess, awsconf, drivLog).Drivers()...)
	drivers = append(drivers, NewLambda(sess, awsconf, drivLog).Drivers()...)
	drivers = append(drivers, NewMonitoring(sess, awsconf, drivLog).Drivers()...)
	drivers = append(drivers, NewCdn(sess, awsconf, drivLog).Drivers()...)
	drivers = append(drivers, NewCloudformation(sess, awsconf, drivLog).Drivers()...)

	return driver.NewMultiDriver(drivers...), nil
}

func initAWSSession(region, profile string, log *logger.Logger) (*session.Session, error) {
	session, err := session.NewSessionWithOptions(session.Options{
		Config: awssdk.Config{
			Region:                        awssdk.String(region),
			HTTPClient:                    &http.Client{Timeout: 2 * time.Second},
			CredentialsChainVerboseErrors: awssdk.Bool(true),
		},
		SharedConfigState:       session.SharedConfigEnable,
		AssumeRoleTokenProvider: stscreds.StdinTokenProvider,
		Profile:                 profile,
	})
	if err != nil {
		return nil, err
	}
	session.Config.Credentials = credentials.NewCredentials(&fileCacheProvider{
		creds:   session.Config.Credentials,
		profile: profile,
		log:     log,
	})
	//session.Config = session.Config.WithLogLevel(awssdk.LogDebugWithHTTPBody)

	if _, err = session.Config.Credentials.Get(); err != nil {
		logCredentialProvidedErrors(log, err)
		return nil, errors.New("Unable to authenticate with neither environment variables, configuration file nor STS credentials. \nExport AWS_ACCESS_KEY_ID and AWS_SECRET_ACCESS_KEY in your CLI environment. Installation documentation is at https://github.com/wallix/awless/wiki/Installation")
	}
	session.Config.HTTPClient = http.DefaultClient

	return session, nil
}

func logCredentialProvidedErrors(log *logger.Logger, err error) {
	if batcherr, ok := err.(awserr.BatchedErrors); ok {
		for _, providerErr := range batcherr.OrigErrs() {
			if baseErr, ok := providerErr.(awserr.Error); ok {
				log.Warningf("%s (err: %s)", baseErr.Message(), baseErr.Code())
				if baseErr.OrigErr() != nil {
					log.ExtraVerbosef("\t%s: %s", baseErr.Code(), strings.Replace(baseErr.OrigErr().Error(), "\n", " ", -1))
				}
			}
		}
	}
}
