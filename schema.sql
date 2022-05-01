-- MariaDB dump 10.19  Distrib 10.6.7-MariaDB, for Linux (x86_64)
--
-- Host: ntp-db-mysql-master.ntpdb.svc.cluster.local    Database: askntp
-- ------------------------------------------------------
-- Server version	5.7.35-38-log

/*!40101 SET @OLD_CHARACTER_SET_CLIENT=@@CHARACTER_SET_CLIENT */;
/*!40101 SET @OLD_CHARACTER_SET_RESULTS=@@CHARACTER_SET_RESULTS */;
/*!40101 SET @OLD_COLLATION_CONNECTION=@@COLLATION_CONNECTION */;
/*!40101 SET NAMES utf8 */;
/*!40103 SET @OLD_TIME_ZONE=@@TIME_ZONE */;
/*!40103 SET TIME_ZONE='+00:00' */;
/*!40014 SET @OLD_UNIQUE_CHECKS=@@UNIQUE_CHECKS, UNIQUE_CHECKS=0 */;
/*!40014 SET @OLD_FOREIGN_KEY_CHECKS=@@FOREIGN_KEY_CHECKS, FOREIGN_KEY_CHECKS=0 */;
/*!40101 SET @OLD_SQL_MODE=@@SQL_MODE, SQL_MODE='NO_AUTO_VALUE_ON_ZERO' */;
/*!40111 SET @OLD_SQL_NOTES=@@SQL_NOTES, SQL_NOTES=0 */;

--
-- Table structure for table `account_invites`
--

DROP TABLE IF EXISTS `account_invites`;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!40101 SET character_set_client = utf8 */;
CREATE TABLE `account_invites` (
  `id` int(10) unsigned NOT NULL AUTO_INCREMENT,
  `account_id` int(10) unsigned NOT NULL,
  `email` varchar(255) NOT NULL,
  `status` enum('pending','accepted','expired') DEFAULT NULL,
  `user_id` int(10) unsigned DEFAULT NULL,
  `sent_by_id` int(10) unsigned NOT NULL,
  `code` varchar(25) NOT NULL,
  `expires_on` datetime NOT NULL,
  `created_on` datetime NOT NULL,
  `modified_on` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `account_id` (`account_id`,`email`),
  UNIQUE KEY `code` (`code`),
  KEY `account_invites_user_fk` (`user_id`),
  KEY `account_invites_sent_by_fk` (`sent_by_id`),
  CONSTRAINT `account_invites_account_fk` FOREIGN KEY (`account_id`) REFERENCES `accounts` (`id`),
  CONSTRAINT `account_invites_sent_by_fk` FOREIGN KEY (`sent_by_id`) REFERENCES `users` (`id`),
  CONSTRAINT `account_invites_user_fk` FOREIGN KEY (`user_id`) REFERENCES `users` (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8;
/*!40101 SET character_set_client = @saved_cs_client */;

--
-- Table structure for table `account_subscriptions`
--

DROP TABLE IF EXISTS `account_subscriptions`;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!40101 SET character_set_client = utf8 */;
CREATE TABLE `account_subscriptions` (
  `id` int(10) unsigned NOT NULL AUTO_INCREMENT,
  `account_id` int(10) unsigned NOT NULL,
  `stripe_subscription_id` varchar(255) DEFAULT NULL,
  `status` enum('incomplete','incomplete_expired','trialing','active','past_due','canceled','unpaid','ended') DEFAULT NULL,
  `name` varchar(255) NOT NULL,
  `max_zones` int(10) unsigned NOT NULL,
  `max_devices` int(10) unsigned NOT NULL,
  `created_on` datetime NOT NULL,
  `ended_on` datetime DEFAULT NULL,
  `modified_on` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `stripe_subscription_id` (`stripe_subscription_id`),
  KEY `account_subscriptions_account_fk` (`account_id`),
  CONSTRAINT `account_subscriptions_account_fk` FOREIGN KEY (`account_id`) REFERENCES `accounts` (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8;
/*!40101 SET character_set_client = @saved_cs_client */;

--
-- Table structure for table `account_users`
--

DROP TABLE IF EXISTS `account_users`;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!40101 SET character_set_client = utf8 */;
CREATE TABLE `account_users` (
  `account_id` int(10) unsigned NOT NULL,
  `user_id` int(10) unsigned NOT NULL,
  PRIMARY KEY (`account_id`,`user_id`),
  KEY `account_users_user_fk` (`user_id`),
  CONSTRAINT `account_users_account_fk` FOREIGN KEY (`account_id`) REFERENCES `accounts` (`id`),
  CONSTRAINT `account_users_user_fk` FOREIGN KEY (`user_id`) REFERENCES `users` (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8;
/*!40101 SET character_set_client = @saved_cs_client */;

--
-- Table structure for table `accounts`
--

DROP TABLE IF EXISTS `accounts`;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!40101 SET character_set_client = utf8 */;
CREATE TABLE `accounts` (
  `id` int(10) unsigned NOT NULL AUTO_INCREMENT,
  `name` varchar(255) DEFAULT NULL,
  `organization_name` varchar(150) DEFAULT NULL,
  `organization_url` varchar(150) DEFAULT NULL,
  `public_profile` tinyint(1) NOT NULL DEFAULT '0',
  `url_slug` varchar(150) DEFAULT NULL,
  `created_on` datetime NOT NULL,
  `modified_on` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  `stripe_customer_id` varchar(255) DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `url_slug_idx` (`url_slug`),
  UNIQUE KEY `stripe_customer_id` (`stripe_customer_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8;
/*!40101 SET character_set_client = @saved_cs_client */;

--
-- Table structure for table `api_keys`
--

DROP TABLE IF EXISTS `api_keys`;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!40101 SET character_set_client = utf8 */;
CREATE TABLE `api_keys` (
  `id` int(10) unsigned NOT NULL AUTO_INCREMENT,
  `api_key` varchar(255) DEFAULT NULL,
  `grants` text,
  `created_on` datetime NOT NULL,
  `modified_on` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `api_key` (`api_key`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8;
/*!40101 SET character_set_client = @saved_cs_client */;

--
-- Table structure for table `combust_cache`
--

DROP TABLE IF EXISTS `combust_cache`;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!40101 SET character_set_client = utf8 */;
CREATE TABLE `combust_cache` (
  `id` varchar(64) NOT NULL,
  `type` varchar(20) NOT NULL DEFAULT '',
  `created` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  `purge_key` varchar(16) DEFAULT NULL,
  `data` mediumblob NOT NULL,
  `metadata` mediumblob,
  `serialized` tinyint(1) NOT NULL DEFAULT '0',
  `expire` datetime NOT NULL DEFAULT '1970-01-01 00:00:00',
  PRIMARY KEY (`id`,`type`),
  KEY `expire_idx` (`expire`),
  KEY `purge_idx` (`purge_key`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8;
/*!40101 SET character_set_client = @saved_cs_client */;

--
-- Table structure for table `combust_secrets`
--

DROP TABLE IF EXISTS `combust_secrets`;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!40101 SET character_set_client = utf8 */;
CREATE TABLE `combust_secrets` (
  `secret_ts` int(10) unsigned NOT NULL,
  `expires_ts` int(10) unsigned NOT NULL,
  `type` varchar(32) NOT NULL,
  `secret` char(32) DEFAULT NULL,
  PRIMARY KEY (`type`,`secret_ts`),
  KEY `expires_ts` (`expires_ts`)
) ENGINE=InnoDB DEFAULT CHARSET=latin1;
/*!40101 SET character_set_client = @saved_cs_client */;

--
-- Table structure for table `dns_roots`
--

DROP TABLE IF EXISTS `dns_roots`;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!40101 SET character_set_client = utf8 */;
CREATE TABLE `dns_roots` (
  `id` int(10) unsigned NOT NULL AUTO_INCREMENT,
  `origin` varchar(255) NOT NULL,
  `vendor_available` tinyint(4) NOT NULL DEFAULT '0',
  `general_use` tinyint(4) NOT NULL DEFAULT '0',
  `ns_list` varchar(255) NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `origin` (`origin`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8;
/*!40101 SET character_set_client = @saved_cs_client */;

--
-- Table structure for table `log_scores`
--

DROP TABLE IF EXISTS `log_scores`;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!40101 SET character_set_client = utf8 */;
CREATE TABLE `log_scores` (
  `id` bigint(20) unsigned NOT NULL AUTO_INCREMENT,
  `monitor_id` int(10) unsigned DEFAULT NULL,
  `server_id` int(10) unsigned NOT NULL,
  `ts` datetime NOT NULL,
  `score` double NOT NULL DEFAULT '0',
  `step` double NOT NULL DEFAULT '0',
  `offset` double DEFAULT NULL,
  `rtt` mediumint(9) DEFAULT NULL,
  `attributes` text,
  PRIMARY KEY (`id`),
  KEY `log_scores_server_ts_idx` (`server_id`,`ts`),
  KEY `ts` (`ts`),
  KEY `log_score_monitor_id_fk` (`monitor_id`),
  CONSTRAINT `log_score_monitor_id_fk` FOREIGN KEY (`monitor_id`) REFERENCES `monitors` (`id`),
  CONSTRAINT `log_scores_server` FOREIGN KEY (`server_id`) REFERENCES `servers` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8;
/*!40101 SET character_set_client = @saved_cs_client */;

--
-- Table structure for table `log_scores_archive_status`
--

DROP TABLE IF EXISTS `log_scores_archive_status`;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!40101 SET character_set_client = utf8 */;
CREATE TABLE `log_scores_archive_status` (
  `id` int(10) unsigned NOT NULL AUTO_INCREMENT,
  `archiver` varchar(255) NOT NULL,
  `log_score_id` bigint(20) unsigned DEFAULT NULL,
  `modified_on` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `archiver` (`archiver`),
  KEY `log_score_id` (`log_score_id`),
  CONSTRAINT `log_score_id` FOREIGN KEY (`log_score_id`) REFERENCES `log_scores` (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8;
/*!40101 SET character_set_client = @saved_cs_client */;

--
-- Table structure for table `log_status`
--

DROP TABLE IF EXISTS `log_status`;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!40101 SET character_set_client = utf8 */;
CREATE TABLE `log_status` (
  `server_id` int(10) unsigned NOT NULL,
  `last_check` datetime NOT NULL,
  `ts_archived` datetime NOT NULL,
  PRIMARY KEY (`server_id`),
  KEY `log_scores_server_ts_idx` (`server_id`,`last_check`),
  KEY `last_check_idx` (`last_check`),
  CONSTRAINT `log_status_server` FOREIGN KEY (`server_id`) REFERENCES `servers` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8;
/*!40101 SET character_set_client = @saved_cs_client */;

--
-- Table structure for table `logs`
--

DROP TABLE IF EXISTS `logs`;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!40101 SET character_set_client = utf8 */;
CREATE TABLE `logs` (
  `id` int(10) unsigned NOT NULL AUTO_INCREMENT,
  `account_id` int(10) unsigned DEFAULT NULL,
  `server_id` int(10) unsigned DEFAULT NULL,
  `user_id` int(10) unsigned DEFAULT NULL,
  `vendor_zone_id` int(10) unsigned DEFAULT NULL,
  `type` varchar(50) DEFAULT NULL,
  `message` text,
  `changes` text,
  `created_on` datetime NOT NULL,
  PRIMARY KEY (`id`),
  KEY `server_id` (`server_id`,`type`),
  KEY `server_logs_user_id` (`user_id`),
  KEY `logs_vendor_zone_id` (`vendor_zone_id`),
  KEY `account_id_idx` (`account_id`),
  CONSTRAINT `account_id_idx` FOREIGN KEY (`account_id`) REFERENCES `accounts` (`id`),
  CONSTRAINT `logs_vendor_zone_id` FOREIGN KEY (`vendor_zone_id`) REFERENCES `vendor_zones` (`id`) ON DELETE CASCADE,
  CONSTRAINT `server_logs_server_id` FOREIGN KEY (`server_id`) REFERENCES `servers` (`id`) ON DELETE CASCADE,
  CONSTRAINT `server_logs_user_id` FOREIGN KEY (`user_id`) REFERENCES `users` (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8;
/*!40101 SET character_set_client = @saved_cs_client */;

--
-- Table structure for table `monitors`
--

DROP TABLE IF EXISTS `monitors`;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!40101 SET character_set_client = utf8 */;
CREATE TABLE `monitors` (
  `id` int(10) unsigned NOT NULL AUTO_INCREMENT,
  `user_id` int(10) unsigned DEFAULT NULL,
  `account_id` int(10) unsigned DEFAULT NULL,
  `name` varchar(30) NOT NULL,
  `location` varchar(255) NOT NULL DEFAULT '',
  `ip` varchar(40) NOT NULL,
  `ip_version` enum('v4','v6') NOT NULL,
  `tls_name` varchar(255) DEFAULT NULL,
  `api_key` varchar(64) DEFAULT NULL,
  `status` enum('pending','testing','active','paused','deleted') NOT NULL,
  `config` text NOT NULL,
  `last_seen` datetime(6) DEFAULT NULL,
  `last_submit` datetime(6) DEFAULT NULL,
  `created_on` datetime NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `ip` (`ip`,`ip_version`),
  UNIQUE KEY `api_key` (`api_key`),
  UNIQUE KEY `monitors_tls_name` (`tls_name`),
  KEY `monitors_user_id` (`user_id`),
  KEY `monitors_account_fk` (`account_id`),
  CONSTRAINT `monitors_account_fk` FOREIGN KEY (`account_id`) REFERENCES `accounts` (`id`),
  CONSTRAINT `monitors_user_id` FOREIGN KEY (`user_id`) REFERENCES `users` (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8;
/*!40101 SET character_set_client = @saved_cs_client */;

--
-- Table structure for table `schema_revision`
--

DROP TABLE IF EXISTS `schema_revision`;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!40101 SET character_set_client = utf8 */;
CREATE TABLE `schema_revision` (
  `revision` smallint(5) unsigned NOT NULL DEFAULT '0',
  `schema_name` varchar(30) NOT NULL,
  PRIMARY KEY (`schema_name`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8;
/*!40101 SET character_set_client = @saved_cs_client */;

--
-- Table structure for table `server_alerts`
--

DROP TABLE IF EXISTS `server_alerts`;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!40101 SET character_set_client = utf8 */;
CREATE TABLE `server_alerts` (
  `server_id` int(10) unsigned NOT NULL,
  `last_score` double NOT NULL,
  `first_email_time` datetime NOT NULL,
  `last_email_time` datetime DEFAULT NULL,
  PRIMARY KEY (`server_id`),
  CONSTRAINT `server_alerts_server` FOREIGN KEY (`server_id`) REFERENCES `servers` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8;
/*!40101 SET character_set_client = @saved_cs_client */;

--
-- Table structure for table `server_notes`
--

DROP TABLE IF EXISTS `server_notes`;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!40101 SET character_set_client = utf8 */;
CREATE TABLE `server_notes` (
  `id` int(10) unsigned NOT NULL AUTO_INCREMENT,
  `server_id` int(10) unsigned NOT NULL,
  `name` varchar(255) NOT NULL DEFAULT '',
  `note` text NOT NULL,
  `created_on` datetime NOT NULL,
  `modified_on` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `server` (`server_id`,`name`),
  KEY `name` (`name`),
  CONSTRAINT `server_notes_ibfk_1` FOREIGN KEY (`server_id`) REFERENCES `servers` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8;
/*!40101 SET character_set_client = @saved_cs_client */;

--
-- Table structure for table `server_scores`
--

DROP TABLE IF EXISTS `server_scores`;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!40101 SET character_set_client = utf8 */;
CREATE TABLE `server_scores` (
  `id` bigint(20) unsigned NOT NULL AUTO_INCREMENT,
  `monitor_id` int(10) unsigned NOT NULL,
  `server_id` int(10) unsigned NOT NULL,
  `score_ts` datetime DEFAULT NULL,
  `score_raw` double NOT NULL DEFAULT '0',
  `stratum` tinyint(3) unsigned DEFAULT NULL,
  `status` enum('inactive','testing','active') NOT NULL DEFAULT 'testing',
  `created_on` datetime NOT NULL,
  `modified_on` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `server_id` (`server_id`,`monitor_id`),
  KEY `monitor_id` (`monitor_id`,`server_id`),
  KEY `monitor_id_2` (`monitor_id`,`score_ts`),
  CONSTRAINT `server_score_monitor_fk` FOREIGN KEY (`monitor_id`) REFERENCES `monitors` (`id`),
  CONSTRAINT `server_score_server_id` FOREIGN KEY (`server_id`) REFERENCES `servers` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8;
/*!40101 SET character_set_client = @saved_cs_client */;

--
-- Table structure for table `server_urls`
--

DROP TABLE IF EXISTS `server_urls`;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!40101 SET character_set_client = utf8 */;
CREATE TABLE `server_urls` (
  `id` int(10) unsigned NOT NULL AUTO_INCREMENT,
  `server_id` int(10) unsigned NOT NULL,
  `url` varchar(255) NOT NULL,
  PRIMARY KEY (`id`),
  KEY `server` (`server_id`),
  CONSTRAINT `server_urls_server` FOREIGN KEY (`server_id`) REFERENCES `servers` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8;
/*!40101 SET character_set_client = @saved_cs_client */;

--
-- Table structure for table `server_zones`
--

DROP TABLE IF EXISTS `server_zones`;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!40101 SET character_set_client = utf8 */;
CREATE TABLE `server_zones` (
  `server_id` int(10) unsigned NOT NULL,
  `zone_id` int(10) unsigned NOT NULL,
  PRIMARY KEY (`server_id`,`zone_id`),
  KEY `locations_zone` (`zone_id`),
  CONSTRAINT `locations_server` FOREIGN KEY (`server_id`) REFERENCES `servers` (`id`) ON DELETE CASCADE,
  CONSTRAINT `locations_zone` FOREIGN KEY (`zone_id`) REFERENCES `zones` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8;
/*!40101 SET character_set_client = @saved_cs_client */;

--
-- Table structure for table `servers`
--

DROP TABLE IF EXISTS `servers`;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!40101 SET character_set_client = utf8 */;
CREATE TABLE `servers` (
  `id` int(10) unsigned NOT NULL AUTO_INCREMENT,
  `ip` varchar(40) NOT NULL,
  `ip_version` enum('v4','v6') NOT NULL DEFAULT 'v4',
  `user_id` int(10) unsigned NOT NULL,
  `account_id` int(10) unsigned DEFAULT NULL,
  `hostname` varchar(255) DEFAULT NULL,
  `stratum` tinyint(3) unsigned DEFAULT NULL,
  `in_pool` tinyint(3) unsigned NOT NULL DEFAULT '0',
  `in_server_list` tinyint(3) unsigned NOT NULL DEFAULT '0',
  `netspeed` mediumint(8) unsigned NOT NULL DEFAULT '1000',
  `created_on` datetime NOT NULL,
  `updated_on` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  `score_ts` datetime DEFAULT NULL,
  `score_raw` double NOT NULL DEFAULT '0',
  `deletion_on` date DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `ip` (`ip`),
  KEY `admin` (`user_id`),
  KEY `score_ts` (`score_ts`),
  KEY `deletion_on` (`deletion_on`),
  KEY `server_account_fk` (`account_id`),
  CONSTRAINT `server_account_fk` FOREIGN KEY (`account_id`) REFERENCES `accounts` (`id`),
  CONSTRAINT `servers_user_ibfk` FOREIGN KEY (`user_id`) REFERENCES `users` (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8;
/*!40101 SET character_set_client = @saved_cs_client */;

--
-- Table structure for table `system_settings`
--

DROP TABLE IF EXISTS `system_settings`;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!40101 SET character_set_client = utf8 */;
CREATE TABLE `system_settings` (
  `id` int(10) unsigned NOT NULL AUTO_INCREMENT,
  `key` varchar(255) DEFAULT NULL,
  `value` text,
  `created_on` datetime NOT NULL,
  `modified_on` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `key` (`key`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8;
/*!40101 SET character_set_client = @saved_cs_client */;

--
-- Table structure for table `user_equipment_applications`
--

DROP TABLE IF EXISTS `user_equipment_applications`;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!40101 SET character_set_client = utf8 */;
CREATE TABLE `user_equipment_applications` (
  `id` int(10) unsigned NOT NULL AUTO_INCREMENT,
  `user_id` int(10) unsigned NOT NULL,
  `application` text,
  `contact_information` text,
  `status` enum('New','Pending','Maybe','No','Approved') NOT NULL DEFAULT 'New',
  PRIMARY KEY (`id`),
  KEY `user_equipment_applications_user_id` (`user_id`),
  CONSTRAINT `user_equipment_applications_user_id` FOREIGN KEY (`user_id`) REFERENCES `users` (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8;
/*!40101 SET character_set_client = @saved_cs_client */;

--
-- Table structure for table `user_identities`
--

DROP TABLE IF EXISTS `user_identities`;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!40101 SET character_set_client = utf8 */;
CREATE TABLE `user_identities` (
  `id` int(10) unsigned NOT NULL AUTO_INCREMENT,
  `profile_id` varchar(255) NOT NULL,
  `user_id` int(10) unsigned NOT NULL,
  `provider` varchar(255) NOT NULL,
  `data` text,
  `email` varchar(255) DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `profile_id` (`profile_id`),
  KEY `user_identities_user_id` (`user_id`),
  CONSTRAINT `user_identities_user_id` FOREIGN KEY (`user_id`) REFERENCES `users` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8;
/*!40101 SET character_set_client = @saved_cs_client */;

--
-- Table structure for table `user_privileges`
--

DROP TABLE IF EXISTS `user_privileges`;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!40101 SET character_set_client = utf8 */;
CREATE TABLE `user_privileges` (
  `user_id` int(10) unsigned NOT NULL,
  `see_all_servers` tinyint(1) NOT NULL DEFAULT '0',
  `see_all_user_profiles` tinyint(1) NOT NULL DEFAULT '0',
  `vendor_admin` tinyint(4) NOT NULL DEFAULT '0',
  `equipment_admin` tinyint(4) NOT NULL DEFAULT '0',
  `support_staff` tinyint(4) NOT NULL DEFAULT '0',
  PRIMARY KEY (`user_id`),
  CONSTRAINT `user_privileges_user` FOREIGN KEY (`user_id`) REFERENCES `users` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8;
/*!40101 SET character_set_client = @saved_cs_client */;

--
-- Table structure for table `users`
--

DROP TABLE IF EXISTS `users`;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!40101 SET character_set_client = utf8 */;
CREATE TABLE `users` (
  `id` int(10) unsigned NOT NULL AUTO_INCREMENT,
  `email` varchar(255) NOT NULL,
  `name` varchar(255) DEFAULT NULL,
  `username` varchar(40) DEFAULT NULL,
  `public_profile` tinyint(1) NOT NULL DEFAULT '0',
  PRIMARY KEY (`id`),
  UNIQUE KEY `email` (`email`),
  UNIQUE KEY `username` (`username`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8;
/*!40101 SET character_set_client = @saved_cs_client */;

--
-- Table structure for table `vendor_zones`
--

DROP TABLE IF EXISTS `vendor_zones`;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!40101 SET character_set_client = utf8 */;
CREATE TABLE `vendor_zones` (
  `id` int(10) unsigned NOT NULL AUTO_INCREMENT,
  `zone_name` varchar(90) NOT NULL,
  `status` enum('New','Pending','Approved','Rejected') NOT NULL DEFAULT 'New',
  `user_id` int(10) unsigned DEFAULT NULL,
  `organization_name` varchar(255) DEFAULT NULL,
  `client_type` enum('ntp','sntp','all') NOT NULL DEFAULT 'ntp',
  `contact_information` text,
  `request_information` text,
  `device_count` int(10) unsigned DEFAULT NULL,
  `rt_ticket` smallint(5) unsigned DEFAULT NULL,
  `approved_on` datetime DEFAULT NULL,
  `created_on` datetime NOT NULL,
  `modified_on` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  `dns_root_id` int(10) unsigned NOT NULL,
  `account_id` int(10) unsigned DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `zone_name` (`zone_name`,`dns_root_id`),
  KEY `vendor_zones_user_id` (`user_id`),
  KEY `dns_root_fk` (`dns_root_id`),
  KEY `vendor_zone_account_fk` (`account_id`),
  CONSTRAINT `dns_root_fk` FOREIGN KEY (`dns_root_id`) REFERENCES `dns_roots` (`id`),
  CONSTRAINT `vendor_zone_account_fk` FOREIGN KEY (`account_id`) REFERENCES `accounts` (`id`),
  CONSTRAINT `vendor_zones_user_id` FOREIGN KEY (`user_id`) REFERENCES `users` (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8;
/*!40101 SET character_set_client = @saved_cs_client */;

--
-- Table structure for table `zone_server_counts`
--

DROP TABLE IF EXISTS `zone_server_counts`;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!40101 SET character_set_client = utf8 */;
CREATE TABLE `zone_server_counts` (
  `id` int(10) unsigned NOT NULL AUTO_INCREMENT,
  `zone_id` int(10) unsigned NOT NULL,
  `ip_version` enum('v4','v6') NOT NULL,
  `date` date NOT NULL,
  `count_active` mediumint(8) unsigned NOT NULL,
  `count_registered` mediumint(8) unsigned NOT NULL,
  `netspeed_active` int(10) unsigned NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `zone_date` (`zone_id`,`date`,`ip_version`),
  CONSTRAINT `zone_server_counts` FOREIGN KEY (`zone_id`) REFERENCES `zones` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8;
/*!40101 SET character_set_client = @saved_cs_client */;

--
-- Table structure for table `zones`
--

DROP TABLE IF EXISTS `zones`;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!40101 SET character_set_client = utf8 */;
CREATE TABLE `zones` (
  `id` int(10) unsigned NOT NULL AUTO_INCREMENT,
  `name` varchar(255) NOT NULL,
  `description` varchar(255) DEFAULT NULL,
  `parent_id` int(10) unsigned DEFAULT NULL,
  `dns` tinyint(1) NOT NULL DEFAULT '1',
  PRIMARY KEY (`id`),
  UNIQUE KEY `name` (`name`),
  KEY `parent` (`parent_id`),
  CONSTRAINT `zones_parent` FOREIGN KEY (`parent_id`) REFERENCES `zones` (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8;
/*!40101 SET character_set_client = @saved_cs_client */;
/*!40103 SET TIME_ZONE=@OLD_TIME_ZONE */;

/*!40101 SET SQL_MODE=@OLD_SQL_MODE */;
/*!40014 SET FOREIGN_KEY_CHECKS=@OLD_FOREIGN_KEY_CHECKS */;
/*!40014 SET UNIQUE_CHECKS=@OLD_UNIQUE_CHECKS */;
/*!40101 SET CHARACTER_SET_CLIENT=@OLD_CHARACTER_SET_CLIENT */;
/*!40101 SET CHARACTER_SET_RESULTS=@OLD_CHARACTER_SET_RESULTS */;
/*!40101 SET COLLATION_CONNECTION=@OLD_COLLATION_CONNECTION */;
/*!40111 SET SQL_NOTES=@OLD_SQL_NOTES */;

-- Dump completed on 2022-05-01 10:40:47
