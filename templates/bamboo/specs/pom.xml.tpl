<!-- Code generated by "welder bamboo-specs" -->
<project xmlns="http://maven.apache.org/POM/4.0.0" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance"
         xsi:schemaLocation="http://maven.apache.org/POM/4.0.0 http://maven.apache.org/maven-v4_0_0.xsd">
    <modelVersion>4.0.0</modelVersion>

    <parent>
        <groupId>com.atlassian.bamboo</groupId>
        <artifactId>bamboo-specs-parent</artifactId>
        <version>8.2.6</version>
        <relativePath/>
    </parent>

    <groupId>{{.PackageName}}</groupId>
    <artifactId>{{.DirectoryName}}</artifactId>
    <version>1.0.0</version>
    <packaging>jar</packaging>

    <properties>
        <kotlin.version>1.8.0</kotlin.version>
    </properties>

    <repositories>
        <repository>
            <id>atlassian-closedsource-snapshots</id>
            <name>atlassian-closedsource-snapshots</name>
            <url>https://packages.atlassian.com/maven-closedsource-snapshot/</url>
            <releases>
                <enabled>false</enabled>
            </releases>
            <snapshots>
                <enabled>true</enabled>
                <updatePolicy>always</updatePolicy>
            </snapshots>
        </repository>
        <repository>
            <id>central</id>
            <name>Central Repository</name>
            <url>https://repo1.maven.org/maven2/</url>
        </repository>
        <repository>
            <id>atlassian-public</id>
            <url>https://packages.atlassian.com/mvn/maven-external/</url>
        </repository>
    </repositories>

    <pluginRepositories>
        <pluginRepository>
            <id>central</id>
            <name>Central Repository</name>
            <url>https://repo1.maven.org/maven2/</url>
        </pluginRepository>
        <pluginRepository>
            <id>atlassian-public</id>
            <url>https://packages.atlassian.com/mvn/maven-external/</url>
        </pluginRepository>
    </pluginRepositories>

    <dependencies>
        <dependency>
            <groupId>com.atlassian.deleng</groupId>
            <artifactId>welder-bamboo-specs</artifactId>
            <version>0.1.0-SNAPSHOT</version>
        </dependency>

        <!-- Test dependencies -->
        <dependency>
            <groupId>junit</groupId>
            <artifactId>junit</artifactId>
            <scope>test</scope>
        </dependency>
    </dependencies>

    <build>
        <plugins>
            <!--Kotlin-->
            <plugin>
                <artifactId>kotlin-maven-plugin</artifactId>
                <groupId>org.jetbrains.kotlin</groupId>
                <version>${kotlin.version}</version>
                <executions>
                    <execution>
                        <id>compile</id>
                        <phase>process-sources</phase>
                        <goals>
                            <goal>compile</goal>
                        </goals>
                    </execution>
                    <execution>
                        <id>test-compile</id>
                        <goals>
                            <goal>test-compile</goal>
                        </goals>
                    </execution>
                </executions>

                <dependencies>
                    <dependency>
                        <groupId>org.jetbrains.kotlin</groupId>
                        <artifactId>kotlin-maven-allopen</artifactId>
                        <version>${kotlin.version}</version>
                    </dependency>
                </dependencies>
            </plugin>

        </plugins>
        <sourceDirectory>${project.basedir}/src/main/kotlin</sourceDirectory>
        <testSourceDirectory>${project.basedir}/src/test/kotlin</testSourceDirectory>
    </build>


    <!-- run 'mvn test' to perform offline validation of the plan -->
    <!-- run 'mvn -Ppublish-specs' to upload the plan to your Bamboo server -->
</project>
