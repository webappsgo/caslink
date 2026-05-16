// Jenkinsfile - Declarative Pipeline for caslink
// See AI.md PART 28 for CI/CD rules

pipeline {
    agent any

    environment {
        PROJECT_NAME = 'caslink'
        PROJECT_ORG  = 'casapps'
        CGO_ENABLED  = '0'
        REGISTRY     = 'ghcr.io/casapps/caslink'
    }

    stages {
        stage('Checkout') {
            steps {
                checkout scm
            }
        }

        stage('Version') {
            steps {
                script {
                    env.VERSION = sh(
                        script: 'cat release.txt 2>/dev/null || echo "0.1.0"',
                        returnStdout: true
                    ).trim()
                    env.COMMIT_ID = sh(
                        script: 'git rev-parse --short HEAD 2>/dev/null || echo "unknown"',
                        returnStdout: true
                    ).trim()
                    env.BUILD_DATE = sh(
                        script: 'date +"%a %b %d, %Y at %H:%M:%S %Z"',
                        returnStdout: true
                    ).trim()
                }
                echo "Building ${env.PROJECT_NAME} v${env.VERSION} (${env.COMMIT_ID})"
            }
        }

        stage('Security') {
            parallel {
                stage('Secret Scan') {
                    steps {
                        sh '''
                            docker run --rm \
                                -v "$(pwd)":/repo \
                                ghcr.io/trufflesecurity/trufflehog:latest \
                                git file:///repo --only-verified --fail || true
                        '''
                    }
                }
                stage('Dependency Check') {
                    steps {
                        sh '''
                            docker run --rm \
                                -v "$(pwd)":/build \
                                -v "$HOME/.cache/go-build":/root/.cache/go-build \
                                -v "$HOME/go/pkg/mod":/go/pkg/mod \
                                -w /build \
                                -e CGO_ENABLED=0 \
                                golang:alpine \
                                sh -c "go install golang.org/x/vuln/cmd/govulncheck@latest && govulncheck ./..."
                        '''
                    }
                }
            }
        }

        stage('Test') {
            steps {
                sh '''
                    docker run --rm \
                        -v "$(pwd)":/build \
                        -v "$HOME/.cache/go-build":/root/.cache/go-build \
                        -v "$HOME/go/pkg/mod":/go/pkg/mod \
                        -w /build \
                        -e CGO_ENABLED=0 \
                        golang:alpine \
                        go test -v -race -coverprofile=coverage.out ./...
                '''
            }
        }

        stage('Build') {
            steps {
                sh '''
                    mkdir -p binaries
                    docker run --rm \
                        -v "$(pwd)":/build \
                        -v "$HOME/.cache/go-build":/root/.cache/go-build \
                        -v "$HOME/go/pkg/mod":/go/pkg/mod \
                        -w /build \
                        -e CGO_ENABLED=0 \
                        golang:alpine \
                        sh -c "
                            LDFLAGS=\"-s -w -X 'main.Version=${VERSION}' -X 'main.CommitID=${COMMIT_ID}' -X 'main.BuildDate=${BUILD_DATE}' -X 'main.OfficialSite=https://caslink.casapps.us'\"
                            for platform in linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64 windows/arm64 freebsd/amd64 freebsd/arm64; do
                                OS=\${platform%/*}
                                ARCH=\${platform#*/}
                                OUTPUT=\"binaries/${PROJECT_NAME}-\${OS}-\${ARCH}\"
                                [ \"\$OS\" = \"windows\" ] && OUTPUT=\"\${OUTPUT}.exe\"
                                echo \"Building \$OS/\$ARCH...\"
                                GOOS=\$OS GOARCH=\$ARCH go build -ldflags \"\$LDFLAGS\" -o \"\$OUTPUT\" ./src
                            done
                        "
                '''
            }
        }

        stage('Docker Image') {
            steps {
                sh '''
                    docker build \
                        --build-arg VERSION="${VERSION}" \
                        --build-arg BUILD_DATE="${BUILD_DATE}" \
                        --build-arg COMMIT_ID="${COMMIT_ID}" \
                        -f docker/Dockerfile \
                        -t "${REGISTRY}:devel" \
                        -t "${REGISTRY}:${COMMIT_ID}" \
                        .
                '''
            }
        }

        stage('Release') {
            when { tag 'v*' }
            steps {
                sh '''
                    # Tag as version and latest
                    docker tag "${REGISTRY}:devel" "${REGISTRY}:${VERSION}"
                    docker tag "${REGISTRY}:devel" "${REGISTRY}:latest"

                    # Generate checksums
                    cd binaries && sha256sum * > checksums.sha256 && cd ..
                '''
            }
        }
    }

    post {
        always {
            cleanWs()
        }
        failure {
            echo "Build failed for ${env.PROJECT_NAME} v${env.VERSION}"
        }
    }
}
