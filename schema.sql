/*M!999999\- enable the sandbox mode */
-- MariaDB dump 10.19-11.4.5-MariaDB, for Linux (x86_64)
--
-- Host: ntpdb-haproxy.ntpdb.svc.cluster.local    Database: askntp
-- ------------------------------------------------------
-- Server version	8.0.40-31

/*!40101 SET @OLD_CHARACTER_SET_CLIENT=@@CHARACTER_SET_CLIENT */;
/*!40101 SET @OLD_CHARACTER_SET_RESULTS=@@CHARACTER_SET_RESULTS */;
/*!40101 SET @OLD_COLLATION_CONNECTION=@@COLLATION_CONNECTION */;
/*!40101 SET NAMES utf8 */;
/*!40103 SET @OLD_TIME_ZONE=@@TIME_ZONE */;
/*!40103 SET TIME_ZONE='+00:00' */;
/*!40014 SET @OLD_UNIQUE_CHECKS=@@UNIQUE_CHECKS, UNIQUE_CHECKS=0 */;
/*!40014 SET @OLD_FOREIGN_KEY_CHECKS=@@FOREIGN_KEY_CHECKS, FOREIGN_KEY_CHECKS=0 */;
/*!40101 SET @OLD_SQL_MODE=@@SQL_MODE, SQL_MODE='NO_AUTO_VALUE_ON_ZERO' */;
/*M!100616 SET @OLD_NOTE_VERBOSITY=@@NOTE_VERBOSITY, NOTE_VERBOSITY=0 */;

--
-- Table structure for table `account_invites`
--

DROP TABLE IF EXISTS `account_invites`;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!40101 SET character_set_client = utf8mb4 */;
CREATE TABLE `account_invites` (
  `id` int unsigned NOT NULL AUTO_INCREMENT,
  `account_id` int unsigned NOT NULL,
  `email` varchar(255) NOT NULL,
  `status` enum('pending','accepted','expired') DEFAULT NULL,
  `user_id` int unsigned DEFAULT NULL,
  `sent_by_id` int unsigned NOT NULL,
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
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb3;
/*!40101 SET character_set_client = @saved_cs_client */;

--
-- Table structure for table `account_subscriptions`
--

DROP TABLE IF EXISTS `account_subscriptions`;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!40101 SET character_set_client = utf8mb4 */;
CREATE TABLE `account_subscriptions` (
  `id` int unsigned NOT NULL AUTO_INCREMENT,
  `account_id` int unsigned NOT NULL,
  `stripe_subscription_id` varchar(255) CHARACTER SET utf8mb3 COLLATE utf8mb3_bin DEFAULT NULL,
  `status` enum('incomplete','incomplete_expired','trialing','active','past_due','canceled','unpaid','ended') DEFAULT NULL,
  `name` varchar(255) NOT NULL,
  `max_zones` int unsigned NOT NULL,
  `max_devices` int unsigned NOT NULL,
  `created_on` datetime NOT NULL,
  `ended_on` datetime DEFAULT NULL,
  `modified_on` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `stripe_subscription_id` (`stripe_subscription_id`),
  KEY `account_subscriptions_account_fk` (`account_id`),
  CONSTRAINT `account_subscriptions_account_fk` FOREIGN KEY (`account_id`) REFERENCES `accounts` (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb3;
/*!40101 SET character_set_client = @saved_cs_client */;

--
-- Table structure for table `account_users`
--

DROP TABLE IF EXISTS `account_users`;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!40101 SET character_set_client = utf8mb4 */;
CREATE TABLE `account_users` (
  `account_id` int unsigned NOT NULL,
  `user_id` int unsigned NOT NULL,
  PRIMARY KEY (`account_id`,`user_id`),
  KEY `account_users_user_fk` (`user_id`),
  CONSTRAINT `account_users_account_fk` FOREIGN KEY (`account_id`) REFERENCES `accounts` (`id`),
  CONSTRAINT `account_users_user_fk` FOREIGN KEY (`user_id`) REFERENCES `users` (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb3;
/*!40101 SET character_set_client = @saved_cs_client */;

--
-- Table structure for table `accounts`
--

DROP TABLE IF EXISTS `accounts`;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!40101 SET character_set_client = utf8mb4 */;
CREATE TABLE `accounts` (
  `id` int unsigned NOT NULL AUTO_INCREMENT,
  `id_token` varchar(36) DEFAULT NULL,
  `name` varchar(255) DEFAULT NULL,
  `organization_name` varchar(150) DEFAULT NULL,
  `organization_url` varchar(150) DEFAULT NULL,
  `public_profile` tinyint(1) NOT NULL DEFAULT '0',
  `url_slug` varchar(150) DEFAULT NULL,
  `flags` json DEFAULT NULL,
  `created_on` datetime NOT NULL,
  `modified_on` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  `stripe_customer_id` varchar(255) CHARACTER SET utf8mb3 COLLATE utf8mb3_bin DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `url_slug_idx` (`url_slug`),
  UNIQUE KEY `stripe_customer_id` (`stripe_customer_id`),
  UNIQUE KEY `id_token` (`id_token`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb3;
/*!40101 SET character_set_client = @saved_cs_client */;

--
-- Table structure for table `api_keys`
--

DROP TABLE IF EXISTS `api_keys`;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!40101 SET character_set_client = utf8mb4 */;
CREATE TABLE `api_keys` (
  `id` int unsigned NOT NULL AUTO_INCREMENT,
  `account_id` int unsigned DEFAULT NULL,
  `user_id` int unsigned DEFAULT NULL,
  `api_key` varchar(255) DEFAULT NULL,
  `grants` text,
  `audience` text NOT NULL,
  `token_lookup` varchar(16) NOT NULL,
  `token_hashed` varchar(256) NOT NULL,
  `last_seen` datetime DEFAULT NULL,
  `created_on` datetime NOT NULL,
  `modified_on` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `api_key` (`api_key`),
  KEY `api_keys_account_fk` (`account_id`),
  KEY `api_keys_user_fk` (`user_id`),
  CONSTRAINT `api_keys_account_fk` FOREIGN KEY (`account_id`) REFERENCES `accounts` (`id`),
  CONSTRAINT `api_keys_user_fk` FOREIGN KEY (`user_id`) REFERENCES `users` (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb3;
/*!40101 SET character_set_client = @saved_cs_client */;

--
-- Table structure for table `api_keys_monitors`
--

DROP TABLE IF EXISTS `api_keys_monitors`;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!40101 SET character_set_client = utf8mb4 */;
CREATE TABLE `api_keys_monitors` (
  `api_key_id` int unsigned NOT NULL,
  `monitor_id` int unsigned NOT NULL,
  PRIMARY KEY (`api_key_id`,`monitor_id`),
  KEY `api_keys_monitors_monitors_fk` (`monitor_id`),
  CONSTRAINT `api_keys_monitors_api_keys_fk` FOREIGN KEY (`api_key_id`) REFERENCES `api_keys` (`id`) ON DELETE CASCADE,
  CONSTRAINT `api_keys_monitors_monitors_fk` FOREIGN KEY (`monitor_id`) REFERENCES `monitors` (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;
/*!40101 SET character_set_client = @saved_cs_client */;

--
-- Table structure for table `combust_cache`
--

DROP TABLE IF EXISTS `combust_cache`;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!40101 SET character_set_client = utf8mb4 */;
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
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb3;
/*!40101 SET character_set_client = @saved_cs_client */;

--
-- Table structure for table `combust_secrets`
--

DROP TABLE IF EXISTS `combust_secrets`;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!40101 SET character_set_client = utf8mb4 */;
CREATE TABLE `combust_secrets` (
  `secret_ts` int unsigned NOT NULL,
  `expires_ts` int unsigned NOT NULL,
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
/*!40101 SET character_set_client = utf8mb4 */;
CREATE TABLE `dns_roots` (
  `id` int unsigned NOT NULL AUTO_INCREMENT,
  `origin` varchar(255) NOT NULL,
  `vendor_available` tinyint NOT NULL DEFAULT '0',
  `general_use` tinyint NOT NULL DEFAULT '0',
  `ns_list` varchar(255) NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `origin` (`origin`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb3;
/*!40101 SET character_set_client = @saved_cs_client */;

--
-- Table structure for table `log_scores`
--

DROP TABLE IF EXISTS `log_scores`;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!40101 SET character_set_client = utf8mb4 */;
CREATE TABLE `log_scores` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `monitor_id` int unsigned DEFAULT NULL,
  `server_id` int unsigned NOT NULL,
  `ts` datetime NOT NULL,
  `score` double NOT NULL DEFAULT '0',
  `step` double NOT NULL DEFAULT '0',
  `offset` double DEFAULT NULL,
  `rtt` mediumint DEFAULT NULL,
  `attributes` text,
  PRIMARY KEY (`id`),
  KEY `log_scores_server_ts_idx` (`server_id`,`ts`),
  KEY `ts` (`ts`),
  KEY `log_score_monitor_id_fk` (`monitor_id`),
  CONSTRAINT `log_score_monitor_id_fk` FOREIGN KEY (`monitor_id`) REFERENCES `monitors` (`id`),
  CONSTRAINT `log_scores_server` FOREIGN KEY (`server_id`) REFERENCES `servers` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb3;
/*!40101 SET character_set_client = @saved_cs_client */;

--
-- Table structure for table `log_scores_archive_status`
--

DROP TABLE IF EXISTS `log_scores_archive_status`;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!40101 SET character_set_client = utf8mb4 */;
CREATE TABLE `log_scores_archive_status` (
  `id` int unsigned NOT NULL AUTO_INCREMENT,
  `archiver` varchar(255) NOT NULL,
  `log_score_id` bigint unsigned DEFAULT NULL,
  `modified_on` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `archiver` (`archiver`),
  KEY `log_score_id` (`log_score_id`),
  CONSTRAINT `log_score_id` FOREIGN KEY (`log_score_id`) REFERENCES `log_scores` (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb3;
/*!40101 SET character_set_client = @saved_cs_client */;

--
-- Table structure for table `logs`
--

DROP TABLE IF EXISTS `logs`;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!40101 SET character_set_client = utf8mb4 */;
CREATE TABLE `logs` (
  `id` int unsigned NOT NULL AUTO_INCREMENT,
  `account_id` int unsigned DEFAULT NULL,
  `server_id` int unsigned DEFAULT NULL,
  `user_id` int unsigned DEFAULT NULL,
  `vendor_zone_id` int unsigned DEFAULT NULL,
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
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb3;
/*!40101 SET character_set_client = @saved_cs_client */;

--
-- Table structure for table `monitor_registrations`
--

DROP TABLE IF EXISTS `monitor_registrations`;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!40101 SET character_set_client = utf8mb4 */;
CREATE TABLE `monitor_registrations` (
  `id` int unsigned NOT NULL AUTO_INCREMENT,
  `monitor_id` int unsigned DEFAULT NULL,
  `request_token` varchar(128) NOT NULL,
  `verification_token` varchar(32) NOT NULL,
  `ip4` varchar(15) NOT NULL DEFAULT '',
  `ip6` varchar(39) NOT NULL DEFAULT '',
  `tls_name` varchar(255) DEFAULT '',
  `hostname` varchar(256) NOT NULL DEFAULT '',
  `location_code` varchar(5) NOT NULL DEFAULT '',
  `account_id` int unsigned DEFAULT NULL,
  `client` varchar(256) NOT NULL DEFAULT '',
  `status` enum('pending','accepted','completed','rejected','cancelled') NOT NULL,
  `last_seen` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `created_on` datetime NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `request_token` (`request_token`),
  UNIQUE KEY `verification_token` (`verification_token`),
  KEY `monitor_registrations_monitor_id_fk` (`monitor_id`),
  KEY `monitor_registrations_account_fk` (`account_id`),
  CONSTRAINT `monitor_registrations_account_fk` FOREIGN KEY (`account_id`) REFERENCES `accounts` (`id`),
  CONSTRAINT `monitor_registrations_monitor_id_fk` FOREIGN KEY (`monitor_id`) REFERENCES `monitors` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;
/*!40101 SET character_set_client = @saved_cs_client */;

--
-- Table structure for table `monitors`
--

DROP TABLE IF EXISTS `monitors`;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!40101 SET character_set_client = utf8mb4 */;
CREATE TABLE `monitors` (
  `id` int unsigned NOT NULL AUTO_INCREMENT,
  `id_token` varchar(36) DEFAULT NULL,
  `type` enum('monitor','score') NOT NULL DEFAULT 'monitor',
  `user_id` int unsigned DEFAULT NULL,
  `account_id` int unsigned DEFAULT NULL,
  `hostname` varchar(255) NOT NULL DEFAULT '',
  `location` varchar(255) NOT NULL DEFAULT '',
  `ip` varchar(40) DEFAULT NULL,
  `ip_version` enum('v4','v6') DEFAULT NULL,
  `tls_name` varchar(255) DEFAULT NULL,
  `api_key` varchar(64) DEFAULT NULL,
  `status` enum('pending','testing','active','paused','deleted') NOT NULL,
  `config` text NOT NULL,
  `client_version` varchar(255) NOT NULL DEFAULT '',
  `last_seen` datetime(6) DEFAULT NULL,
  `last_submit` datetime(6) DEFAULT NULL,
  `created_on` datetime NOT NULL,
  `deleted_on` datetime DEFAULT NULL,
  `is_current` tinyint(1) DEFAULT '1',
  PRIMARY KEY (`id`),
  UNIQUE KEY `api_key` (`api_key`),
  UNIQUE KEY `monitors_tls_name` (`tls_name`,`ip_version`),
  UNIQUE KEY `token_id` (`id_token`),
  UNIQUE KEY `id_token` (`id_token`),
  UNIQUE KEY `ip` (`ip`,`is_current`),
  KEY `monitors_user_id` (`user_id`),
  KEY `monitors_account_fk` (`account_id`),
  KEY `type_status` (`type`,`status`),
  CONSTRAINT `monitors_account_fk` FOREIGN KEY (`account_id`) REFERENCES `accounts` (`id`),
  CONSTRAINT `monitors_user_id` FOREIGN KEY (`user_id`) REFERENCES `users` (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb3;
/*!40101 SET character_set_client = @saved_cs_client */;

--
-- Temporary table structure for view `monitors_data`
--

DROP TABLE IF EXISTS `monitors_data`;
/*!50001 DROP VIEW IF EXISTS `monitors_data`*/;
SET @saved_cs_client     = @@character_set_client;
SET character_set_client = utf8mb4;
/*!50001 CREATE VIEW `monitors_data` AS SELECT
 1 AS `id`,
  1 AS `account_id`,
  1 AS `type`,
  1 AS `name`,
  1 AS `ip`,
  1 AS `ip_version`,
  1 AS `status`,
  1 AS `client_version`,
  1 AS `last_seen`,
  1 AS `last_submit` */;
SET character_set_client = @saved_cs_client;

--
-- Table structure for table `schema_revision`
--

DROP TABLE IF EXISTS `schema_revision`;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!40101 SET character_set_client = utf8mb4 */;
CREATE TABLE `schema_revision` (
  `revision` smallint unsigned NOT NULL DEFAULT '0',
  `schema_name` varchar(30) NOT NULL,
  PRIMARY KEY (`schema_name`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb3;
/*!40101 SET character_set_client = @saved_cs_client */;

--
-- Table structure for table `scorer_status`
--

DROP TABLE IF EXISTS `scorer_status`;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!40101 SET character_set_client = utf8mb4 */;
CREATE TABLE `scorer_status` (
  `id` int unsigned NOT NULL AUTO_INCREMENT,
  `scorer_id` int unsigned NOT NULL,
  `log_score_id` bigint unsigned NOT NULL,
  `modified_on` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  KEY `scorer_log_score_id` (`log_score_id`),
  KEY `scores_status_monitor_id_fk` (`scorer_id`),
  CONSTRAINT `scorer_log_score_id` FOREIGN KEY (`log_score_id`) REFERENCES `log_scores` (`id`),
  CONSTRAINT `scores_status_monitor_id_fk` FOREIGN KEY (`scorer_id`) REFERENCES `monitors` (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb3;
/*!40101 SET character_set_client = @saved_cs_client */;

--
-- Table structure for table `server_alerts`
--

DROP TABLE IF EXISTS `server_alerts`;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!40101 SET character_set_client = utf8mb4 */;
CREATE TABLE `server_alerts` (
  `server_id` int unsigned NOT NULL,
  `last_score` double NOT NULL,
  `first_email_time` datetime NOT NULL,
  `last_email_time` datetime DEFAULT NULL,
  PRIMARY KEY (`server_id`),
  CONSTRAINT `server_alerts_server` FOREIGN KEY (`server_id`) REFERENCES `servers` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb3;
/*!40101 SET character_set_client = @saved_cs_client */;

--
-- Table structure for table `server_notes`
--

DROP TABLE IF EXISTS `server_notes`;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!40101 SET character_set_client = utf8mb4 */;
CREATE TABLE `server_notes` (
  `id` int unsigned NOT NULL AUTO_INCREMENT,
  `server_id` int unsigned NOT NULL,
  `name` varchar(255) NOT NULL DEFAULT '',
  `note` text NOT NULL,
  `created_on` datetime NOT NULL,
  `modified_on` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `server` (`server_id`,`name`),
  KEY `name` (`name`),
  CONSTRAINT `server_notes_ibfk_1` FOREIGN KEY (`server_id`) REFERENCES `servers` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb3;
/*!40101 SET character_set_client = @saved_cs_client */;

--
-- Table structure for table `server_scores`
--

DROP TABLE IF EXISTS `server_scores`;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!40101 SET character_set_client = utf8mb4 */;
CREATE TABLE `server_scores` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `monitor_id` int unsigned NOT NULL,
  `server_id` int unsigned NOT NULL,
  `score_ts` datetime DEFAULT NULL,
  `score_raw` double NOT NULL DEFAULT '0',
  `stratum` tinyint unsigned DEFAULT NULL,
  `status` enum('new','candidate','testing','active') NOT NULL DEFAULT 'new',
  `queue_ts` datetime DEFAULT NULL,
  `created_on` datetime NOT NULL,
  `modified_on` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  `constraint_violation_type` varchar(50) DEFAULT NULL,
  `constraint_violation_since` datetime DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `server_id` (`server_id`,`monitor_id`),
  KEY `monitor_id` (`monitor_id`,`server_id`),
  KEY `monitor_id_2` (`monitor_id`,`score_ts`),
  KEY `idx_constraint_violation` (`constraint_violation_type`,`constraint_violation_since`),
  CONSTRAINT `server_score_monitor_fk` FOREIGN KEY (`monitor_id`) REFERENCES `monitors` (`id`),
  CONSTRAINT `server_score_server_id` FOREIGN KEY (`server_id`) REFERENCES `servers` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb3;
/*!40101 SET character_set_client = @saved_cs_client */;

--
-- Table structure for table `server_urls`
--

DROP TABLE IF EXISTS `server_urls`;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!40101 SET character_set_client = utf8mb4 */;
CREATE TABLE `server_urls` (
  `id` int unsigned NOT NULL AUTO_INCREMENT,
  `server_id` int unsigned NOT NULL,
  `url` varchar(255) NOT NULL,
  PRIMARY KEY (`id`),
  KEY `server` (`server_id`),
  CONSTRAINT `server_urls_server` FOREIGN KEY (`server_id`) REFERENCES `servers` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb3;
/*!40101 SET character_set_client = @saved_cs_client */;

--
-- Table structure for table `server_verifications`
--

DROP TABLE IF EXISTS `server_verifications`;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!40101 SET character_set_client = utf8mb4 */;
CREATE TABLE `server_verifications` (
  `id` int unsigned NOT NULL AUTO_INCREMENT,
  `server_id` int unsigned NOT NULL,
  `user_id` int unsigned DEFAULT NULL,
  `user_ip` varchar(45) NOT NULL DEFAULT '',
  `indirect_ip` varchar(45) NOT NULL DEFAULT '',
  `verified_on` datetime DEFAULT NULL,
  `token` varchar(36) DEFAULT NULL,
  `created_on` datetime NOT NULL,
  `modified_on` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `server` (`server_id`),
  UNIQUE KEY `token` (`token`),
  KEY `server_verifications_ibfk_2` (`user_id`),
  CONSTRAINT `server_verifications_ibfk_1` FOREIGN KEY (`server_id`) REFERENCES `servers` (`id`) ON DELETE CASCADE,
  CONSTRAINT `server_verifications_ibfk_2` FOREIGN KEY (`user_id`) REFERENCES `users` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb3;
/*!40101 SET character_set_client = @saved_cs_client */;

--
-- Table structure for table `server_verifications_history`
--

DROP TABLE IF EXISTS `server_verifications_history`;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!40101 SET character_set_client = utf8mb4 */;
CREATE TABLE `server_verifications_history` (
  `id` int unsigned NOT NULL AUTO_INCREMENT,
  `server_id` int unsigned NOT NULL,
  `user_id` int unsigned DEFAULT NULL,
  `user_ip` varchar(45) NOT NULL DEFAULT '',
  `indirect_ip` varchar(45) NOT NULL DEFAULT '',
  `verified_on` datetime DEFAULT NULL,
  `created_on` datetime NOT NULL,
  `modified_on` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  KEY `server_verifications_history_ibfk_1` (`server_id`),
  KEY `server_verifications_history_ibfk_2` (`user_id`),
  CONSTRAINT `server_verifications_history_ibfk_1` FOREIGN KEY (`server_id`) REFERENCES `servers` (`id`) ON DELETE CASCADE,
  CONSTRAINT `server_verifications_history_ibfk_2` FOREIGN KEY (`user_id`) REFERENCES `users` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb3;
/*!40101 SET character_set_client = @saved_cs_client */;

--
-- Table structure for table `server_zones`
--

DROP TABLE IF EXISTS `server_zones`;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!40101 SET character_set_client = utf8mb4 */;
CREATE TABLE `server_zones` (
  `server_id` int unsigned NOT NULL,
  `zone_id` int unsigned NOT NULL,
  PRIMARY KEY (`server_id`,`zone_id`),
  KEY `locations_zone` (`zone_id`),
  CONSTRAINT `locations_server` FOREIGN KEY (`server_id`) REFERENCES `servers` (`id`) ON DELETE CASCADE,
  CONSTRAINT `locations_zone` FOREIGN KEY (`zone_id`) REFERENCES `zones` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb3;
/*!40101 SET character_set_client = @saved_cs_client */;

--
-- Table structure for table `servers`
--

DROP TABLE IF EXISTS `servers`;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!40101 SET character_set_client = utf8mb4 */;
CREATE TABLE `servers` (
  `id` int unsigned NOT NULL AUTO_INCREMENT,
  `ip` varchar(40) NOT NULL,
  `ip_version` enum('v4','v6') NOT NULL DEFAULT 'v4',
  `user_id` int unsigned DEFAULT NULL,
  `account_id` int unsigned DEFAULT NULL,
  `hostname` varchar(255) DEFAULT NULL,
  `stratum` tinyint unsigned DEFAULT NULL,
  `in_pool` tinyint unsigned NOT NULL DEFAULT '0',
  `in_server_list` tinyint unsigned NOT NULL DEFAULT '0',
  `netspeed` int unsigned NOT NULL DEFAULT '10000',
  `netspeed_target` int unsigned NOT NULL DEFAULT '10000',
  `created_on` datetime NOT NULL,
  `updated_on` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  `score_ts` datetime DEFAULT NULL,
  `score_raw` double NOT NULL DEFAULT '0',
  `deletion_on` date DEFAULT NULL,
  `flags` varchar(4096) NOT NULL DEFAULT '{}',
  PRIMARY KEY (`id`),
  UNIQUE KEY `ip` (`ip`),
  KEY `admin` (`user_id`),
  KEY `score_ts` (`score_ts`),
  KEY `deletion_on` (`deletion_on`),
  KEY `server_account_fk` (`account_id`),
  CONSTRAINT `server_account_fk` FOREIGN KEY (`account_id`) REFERENCES `accounts` (`id`),
  CONSTRAINT `servers_user_ibfk` FOREIGN KEY (`user_id`) REFERENCES `users` (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb3;
/*!40101 SET character_set_client = @saved_cs_client */;

--
-- Table structure for table `servers_monitor_review`
--

DROP TABLE IF EXISTS `servers_monitor_review`;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!40101 SET character_set_client = utf8mb4 */;
CREATE TABLE `servers_monitor_review` (
  `server_id` int unsigned NOT NULL,
  `last_review` datetime DEFAULT NULL,
  `next_review` datetime DEFAULT NULL,
  `last_change` datetime DEFAULT NULL,
  `config` varchar(4096) NOT NULL DEFAULT '',
  PRIMARY KEY (`server_id`),
  KEY `next_review` (`next_review`),
  CONSTRAINT `server_monitor_review_server_id_fk` FOREIGN KEY (`server_id`) REFERENCES `servers` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb3;
/*!40101 SET character_set_client = @saved_cs_client */;

--
-- Table structure for table `system_settings`
--

DROP TABLE IF EXISTS `system_settings`;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!40101 SET character_set_client = utf8mb4 */;
CREATE TABLE `system_settings` (
  `id` int unsigned NOT NULL AUTO_INCREMENT,
  `key` varchar(255) NOT NULL,
  `value` text NOT NULL,
  `created_on` datetime NOT NULL,
  `modified_on` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `key` (`key`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb3;
/*!40101 SET character_set_client = @saved_cs_client */;

--
-- Table structure for table `user_equipment_applications`
--

DROP TABLE IF EXISTS `user_equipment_applications`;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!40101 SET character_set_client = utf8mb4 */;
CREATE TABLE `user_equipment_applications` (
  `id` int unsigned NOT NULL AUTO_INCREMENT,
  `user_id` int unsigned NOT NULL,
  `application` text,
  `contact_information` text,
  `status` enum('New','Pending','Maybe','No','Approved') NOT NULL DEFAULT 'New',
  PRIMARY KEY (`id`),
  KEY `user_equipment_applications_user_id` (`user_id`),
  CONSTRAINT `user_equipment_applications_user_id` FOREIGN KEY (`user_id`) REFERENCES `users` (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb3;
/*!40101 SET character_set_client = @saved_cs_client */;

--
-- Table structure for table `user_identities`
--

DROP TABLE IF EXISTS `user_identities`;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!40101 SET character_set_client = utf8mb4 */;
CREATE TABLE `user_identities` (
  `id` int unsigned NOT NULL AUTO_INCREMENT,
  `profile_id` varchar(255) NOT NULL,
  `user_id` int unsigned NOT NULL,
  `provider` varchar(255) NOT NULL,
  `data` text,
  `email` varchar(255) DEFAULT NULL,
  `created_on` datetime NOT NULL DEFAULT '2003-01-27 00:00:00',
  `modified_on` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `profile_id` (`profile_id`),
  KEY `user_identities_user_id` (`user_id`),
  CONSTRAINT `user_identities_user_id` FOREIGN KEY (`user_id`) REFERENCES `users` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb3;
/*!40101 SET character_set_client = @saved_cs_client */;

--
-- Table structure for table `user_privileges`
--

DROP TABLE IF EXISTS `user_privileges`;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!40101 SET character_set_client = utf8mb4 */;
CREATE TABLE `user_privileges` (
  `user_id` int unsigned NOT NULL,
  `see_all_servers` tinyint(1) NOT NULL DEFAULT '0',
  `vendor_admin` tinyint NOT NULL DEFAULT '0',
  `equipment_admin` tinyint NOT NULL DEFAULT '0',
  `support_staff` tinyint NOT NULL DEFAULT '0',
  `monitor_admin` tinyint NOT NULL DEFAULT '0',
  PRIMARY KEY (`user_id`),
  CONSTRAINT `user_privileges_user` FOREIGN KEY (`user_id`) REFERENCES `users` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb3;
/*!40101 SET character_set_client = @saved_cs_client */;

--
-- Table structure for table `user_sessions`
--

DROP TABLE IF EXISTS `user_sessions`;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!40101 SET character_set_client = utf8mb4 */;
CREATE TABLE `user_sessions` (
  `id` int unsigned NOT NULL AUTO_INCREMENT,
  `user_id` int unsigned NOT NULL,
  `token_lookup` varchar(16) NOT NULL,
  `token_hashed` varchar(256) NOT NULL,
  `last_seen` datetime DEFAULT NULL,
  `created_on` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  KEY `user_sessions_user_fk` (`user_id`),
  KEY `token_lookup` (`token_lookup`),
  CONSTRAINT `user_sessions_user_fk` FOREIGN KEY (`user_id`) REFERENCES `users` (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;
/*!40101 SET character_set_client = @saved_cs_client */;

--
-- Table structure for table `user_tasks`
--

DROP TABLE IF EXISTS `user_tasks`;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!40101 SET character_set_client = utf8mb4 */;
CREATE TABLE `user_tasks` (
  `id` int unsigned NOT NULL AUTO_INCREMENT,
  `user_id` int unsigned DEFAULT NULL,
  `task` enum('download','delete') NOT NULL,
  `status` text NOT NULL,
  `traceid` varchar(32) NOT NULL DEFAULT '',
  `execute_on` datetime DEFAULT NULL,
  `created_on` datetime NOT NULL,
  `modified_on` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  KEY `user_tasks_user_fk` (`user_id`),
  CONSTRAINT `user_tasks_user_fk` FOREIGN KEY (`user_id`) REFERENCES `users` (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb3;
/*!40101 SET character_set_client = @saved_cs_client */;

--
-- Table structure for table `users`
--

DROP TABLE IF EXISTS `users`;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!40101 SET character_set_client = utf8mb4 */;
CREATE TABLE `users` (
  `id` int unsigned NOT NULL AUTO_INCREMENT,
  `id_token` varchar(36) DEFAULT NULL,
  `email` varchar(255) NOT NULL,
  `name` varchar(255) DEFAULT NULL,
  `username` varchar(40) DEFAULT NULL,
  `public_profile` tinyint(1) NOT NULL DEFAULT '0',
  `deletion_on` datetime DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `email` (`email`),
  UNIQUE KEY `username` (`username`),
  UNIQUE KEY `id_token` (`id_token`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb3;
/*!40101 SET character_set_client = @saved_cs_client */;

--
-- Table structure for table `vendor_zones`
--

DROP TABLE IF EXISTS `vendor_zones`;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!40101 SET character_set_client = utf8mb4 */;
CREATE TABLE `vendor_zones` (
  `id` int unsigned NOT NULL AUTO_INCREMENT,
  `id_token` varchar(36) DEFAULT NULL,
  `zone_name` varchar(90) NOT NULL,
  `status` enum('New','Pending','Approved','Rejected') NOT NULL DEFAULT 'New',
  `user_id` int unsigned DEFAULT NULL,
  `organization_name` varchar(255) DEFAULT NULL,
  `client_type` enum('ntp','sntp','legacy') NOT NULL DEFAULT 'sntp',
  `contact_information` text,
  `request_information` text,
  `device_information` text,
  `device_count` int unsigned DEFAULT NULL,
  `opensource` tinyint(1) NOT NULL DEFAULT '0',
  `opensource_info` text,
  `rt_ticket` smallint unsigned DEFAULT NULL,
  `approved_on` datetime DEFAULT NULL,
  `created_on` datetime NOT NULL,
  `modified_on` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  `dns_root_id` int unsigned NOT NULL,
  `account_id` int unsigned DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `zone_name` (`zone_name`,`dns_root_id`),
  UNIQUE KEY `id_token` (`id_token`),
  KEY `vendor_zones_user_id` (`user_id`),
  KEY `dns_root_fk` (`dns_root_id`),
  KEY `vendor_zone_account_fk` (`account_id`),
  CONSTRAINT `dns_root_fk` FOREIGN KEY (`dns_root_id`) REFERENCES `dns_roots` (`id`),
  CONSTRAINT `vendor_zone_account_fk` FOREIGN KEY (`account_id`) REFERENCES `accounts` (`id`),
  CONSTRAINT `vendor_zones_user_id` FOREIGN KEY (`user_id`) REFERENCES `users` (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb3;
/*!40101 SET character_set_client = @saved_cs_client */;

--
-- Table structure for table `zone_server_counts`
--

DROP TABLE IF EXISTS `zone_server_counts`;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!40101 SET character_set_client = utf8mb4 */;
CREATE TABLE `zone_server_counts` (
  `id` int unsigned NOT NULL AUTO_INCREMENT,
  `zone_id` int unsigned NOT NULL,
  `ip_version` enum('v4','v6') NOT NULL,
  `date` date NOT NULL,
  `count_active` mediumint unsigned NOT NULL,
  `count_registered` mediumint unsigned NOT NULL,
  `netspeed_active` int unsigned NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `zone_date` (`zone_id`,`date`,`ip_version`),
  KEY `date_idx` (`date`,`zone_id`),
  CONSTRAINT `zone_server_counts` FOREIGN KEY (`zone_id`) REFERENCES `zones` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb3;
/*!40101 SET character_set_client = @saved_cs_client */;

--
-- Table structure for table `zones`
--

DROP TABLE IF EXISTS `zones`;
/*!40101 SET @saved_cs_client     = @@character_set_client */;
/*!40101 SET character_set_client = utf8mb4 */;
CREATE TABLE `zones` (
  `id` int unsigned NOT NULL AUTO_INCREMENT,
  `name` varchar(255) NOT NULL,
  `description` varchar(255) DEFAULT NULL,
  `parent_id` int unsigned DEFAULT NULL,
  `dns` tinyint(1) NOT NULL DEFAULT '1',
  PRIMARY KEY (`id`),
  UNIQUE KEY `name` (`name`),
  KEY `parent` (`parent_id`),
  CONSTRAINT `zones_parent` FOREIGN KEY (`parent_id`) REFERENCES `zones` (`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb3;
/*!40101 SET character_set_client = @saved_cs_client */;

--
-- Final view structure for view `monitors_data`
--

/*!50001 DROP VIEW IF EXISTS `monitors_data`*/;
/*!50001 SET @saved_cs_client          = @@character_set_client */;
/*!50001 SET @saved_cs_results         = @@character_set_results */;
/*!50001 SET @saved_col_connection     = @@collation_connection */;
/*!50001 SET character_set_client      = utf8mb4 */;
/*!50001 SET character_set_results     = utf8mb4 */;
/*!50001 SET collation_connection      = utf8mb4_general_ci */;
/*!50001 CREATE ALGORITHM=UNDEFINED */

/*!50001 VIEW `monitors_data` AS select `monitors`.`id` AS `id`,`monitors`.`account_id` AS `account_id`,`monitors`.`type` AS `type`,if((`monitors`.`type` = 'score'),`monitors`.`hostname`,substring_index(`monitors`.`tls_name`,'.',1)) AS `name`,`monitors`.`ip` AS `ip`,`monitors`.`ip_version` AS `ip_version`,`monitors`.`status` AS `status`,`monitors`.`client_version` AS `client_version`,`monitors`.`last_seen` AS `last_seen`,`monitors`.`last_submit` AS `last_submit` from `monitors` where (not((`monitors`.`tls_name` like '%.system'))) */;
/*!50001 SET character_set_client      = @saved_cs_client */;
/*!50001 SET character_set_results     = @saved_cs_results */;
/*!50001 SET collation_connection      = @saved_col_connection */;
/*!40103 SET TIME_ZONE=@OLD_TIME_ZONE */;

/*!40101 SET SQL_MODE=@OLD_SQL_MODE */;
/*!40014 SET FOREIGN_KEY_CHECKS=@OLD_FOREIGN_KEY_CHECKS */;
/*!40014 SET UNIQUE_CHECKS=@OLD_UNIQUE_CHECKS */;
/*!40101 SET CHARACTER_SET_CLIENT=@OLD_CHARACTER_SET_CLIENT */;
/*!40101 SET CHARACTER_SET_RESULTS=@OLD_CHARACTER_SET_RESULTS */;
/*!40101 SET COLLATION_CONNECTION=@OLD_COLLATION_CONNECTION */;
/*M!100616 SET NOTE_VERBOSITY=@OLD_NOTE_VERBOSITY */;

-- Dump completed on 2025-06-22 21:17:53
