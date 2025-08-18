package server

import (
	"net/http"

	"distributed-job-processor/internal/election"
	"distributed-job-processor/internal/storage"

	"github.com/gin-gonic/gin"
)

func (s *Server) getElectionInfo(c *gin.Context) {
	factory := election.NewElectionFactory()
	
	response := gin.H{
		"current_algorithm": s.config.Election.Algorithm,
		"is_leader":        s.election.IsLeader(),
		"leader_id":        s.election.GetLeader(),
		"node_id":          s.config.Server.NodeID,
		"supported_algorithms": factory.GetSupportedAlgorithms(),
	}
	
	c.JSON(http.StatusOK, response)
}

func (s *Server) getAlgorithmInfo(c *gin.Context) {
	algorithm := c.Param("algorithm")
	factory := election.NewElectionFactory()
	
	description := factory.GetAlgorithmDescription(algorithm)
	if description == "Unknown algorithm" {
		c.JSON(http.StatusNotFound, gin.H{"error": "Algorithm not found"})
		return
	}
	
	response := gin.H{
		"algorithm":     algorithm,
		"description":   description,
		"configuration": factory.GetRecommendedConfiguration(algorithm, 5),
	}
	
	c.JSON(http.StatusOK, response)
}

func (s *Server) getClusterStatus(c *gin.Context) {
	nodes, err := s.storage.GetNodes(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	
	var leaderNode *storage.NodeInfo
	aliveNodes := 0
	
	for _, node := range nodes {
		if node.IsLeader {
			leaderNode = node
		}
		aliveNodes++
	}
	
	response := gin.H{
		"cluster_size":       aliveNodes,
		"leader":            leaderNode,
		"election_algorithm": s.config.Election.Algorithm,
		"nodes":             nodes,
	}
	
	c.JSON(http.StatusOK, response)
}